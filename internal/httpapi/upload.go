package httpapi

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"go-learn/internal/store"
	"go-learn/internal/text"
)

const multipartOverheadBytes = 2 * 1024 * 1024

func uploadFileHandler(d RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Store == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "store not initialized", "")
			return
		}
		if d.UploadSem == nil {
			Error(w, http.StatusInternalServerError, "INTERNAL", "upload limiter not initialized", "")
			return
		}
		if d.MaxFileBytes <= 0 {
			Error(w, http.StatusInternalServerError, "INTERNAL", "max file size not initialized", "")
			return
		}

		if r.ContentLength < 0 {
			Error(w, http.StatusLengthRequired, "LENGTH_REQUIRED", "为保证内存可控，需要 Content-Length", "")
			return
		}

		maxRequest := d.MaxRequestBytes
		if maxRequest <= 0 {
			maxRequest = d.MaxFileBytes + multipartOverheadBytes
		}
		if r.ContentLength > maxRequest {
			Error(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "请求体过大", "超出允许的上传大小")
			return
		}

		if !d.UploadSem.TryAcquire() {
			w.Header().Set("Retry-After", "1")
			Error(w, http.StatusServiceUnavailable, "BUSY", "上传并发已满，请稍后重试", "")
			return
		}
		defer d.UploadSem.Release()

		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "仅支持 multipart/form-data 上传", "")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequest)

		mr, err := r.MultipartReader()
		if err != nil {
			if isMaxBytesError(err) {
				Error(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "请求体过大", "")
				return
			}
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "无法解析 multipart 数据", err.Error())
			return
		}

		part, fileName, err := readFilePart(mr)
		if err != nil {
			if isMaxBytesError(err) {
				Error(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "请求体过大", "")
				return
			}
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "上传数据不合法", err.Error())
			return
		}
		defer part.Close()

		if d.Store.HasName(fileName) {
			Error(w, http.StatusConflict, "NAME_CONFLICT", "重名", "")
			return
		}

		// 上传前的“最佳努力”预淘汰：使用 Content-Length 作为上界估算，尽量降低读取大文件前的内存压力。
		estimated := r.ContentLength
		if estimated <= 0 || estimated > d.MaxFileBytes {
			estimated = d.MaxFileBytes
		}
		if err := d.Store.EvictToFit(estimated); err != nil {
			if errors.Is(err, store.ErrInsufficientSpace) || errors.Is(err, store.ErrTooLarge) {
				Error(w, http.StatusInsufficientStorage, "INSUFFICIENT_STORAGE", "空间不足，无法接收该上传", "")
				return
			}
			Error(w, http.StatusInternalServerError, "INTERNAL", "预处理失败", err.Error())
			return
		}

		data, err := readAtMost(part, d.MaxFileBytes)
		if err != nil {
			if errors.Is(err, store.ErrTooLarge) {
				Error(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "文件过大", "")
				return
			}
			if isMaxBytesError(err) {
				Error(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "请求体过大", "")
				return
			}
			Error(w, http.StatusBadRequest, "BAD_REQUEST", "读取上传内容失败", err.Error())
			return
		}

		isText, enc := text.DetectTextAndEncoding(data)

		meta, err := d.Store.Add(store.AddParams{
			Name:     fileName,
			Bytes:    data,
			Encoding: enc,
			IsText:   isText,
			Now:      time.Now(),
		})
		if err != nil {
			switch {
			case errors.Is(err, store.ErrNameConflict):
				Error(w, http.StatusConflict, "NAME_CONFLICT", "重名", "")
			case errors.Is(err, store.ErrTooLarge):
				Error(w, http.StatusInsufficientStorage, "INSUFFICIENT_STORAGE", "超过总内存上限，无法保存该文件", "")
			case errors.Is(err, store.ErrInsufficientSpace):
				Error(w, http.StatusInsufficientStorage, "INSUFFICIENT_STORAGE", "空间不足，无法保存该文件", "")
			default:
				Error(w, http.StatusInternalServerError, "INTERNAL", "保存失败", err.Error())
			}
			return
		}

		JSON(w, http.StatusCreated, fileListItem{
			ID:        meta.ID,
			Name:      meta.Name,
			CreatedAt: meta.CreatedAt,
			SizeBytes: meta.SizeBytes,
			Encoding:  normalizeEncoding(meta.Encoding),
			IsText:    meta.IsText,
		})
	}
}

func readFilePart(mr *multipart.Reader) (*multipart.Part, string, error) {
	for {
		part, err := mr.NextPart()
		if err != nil {
			if err == io.EOF {
				return nil, "", errors.New("missing file part")
			}
			return nil, "", err
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		if part.FileName() == "" {
			_ = part.Close()
			return nil, "", errors.New("file name is empty")
		}
		return part, part.FileName(), nil
	}
}

func readAtMost(r io.Reader, max int64) ([]byte, error) {
	limited := io.LimitReader(r, max+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, store.ErrTooLarge
	}
	return b, nil
}

func isMaxBytesError(err error) bool {
	if err == nil {
		return false
	}
	// http.MaxBytesReader returns *http.MaxBytesError
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return true
	}
	// 兼容部分 multipart 报错路径
	if strings.Contains(err.Error(), "http: request body too large") {
		return true
	}
	// 有些实现会包一层
	if strings.Contains(err.Error(), "max bytes") {
		return true
	}
	return false
}
