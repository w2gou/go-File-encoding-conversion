(() => {
  const ENCODINGS = ["UTF-8", "GB18030", "GBK", "Big5", "Windows-1252", "ISO-8859-1"];
  let selectedFileIdForBridgeDownload = "";

  const uploadForm = document.getElementById("upload-form");
  const uploadMsg = document.getElementById("upload-msg");
  const refreshBtn = document.getElementById("refresh-btn");
  const filesBody = document.getElementById("files-body");
  const listMsg = document.getElementById("list-msg");
  const bridgeUploadBtn = document.getElementById("bridge-upload-btn");
  const bridgeDownloadBtn = document.getElementById("bridge-download-btn");
  const qrMsg = document.getElementById("qr-msg");
  const qrBox = document.getElementById("qr-box");
  const qrImg = document.getElementById("qr-img");
  const qrLink = document.getElementById("qr-link");

  function setMsg(el, text) {
    el.textContent = text || "";
  }

  async function requestJSON(url, init) {
    const res = await fetch(url, init);
    const text = await res.text();
    let data = null;
    if (text) {
      try {
        data = JSON.parse(text);
      } catch (_) {
        data = { message: text };
      }
    }
    if (!res.ok) {
      const message = data && data.message ? data.message : `HTTP ${res.status}`;
      throw new Error(message);
    }
    return data;
  }

  function sizeText(n) {
    if (typeof n !== "number") return "-";
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / (1024 * 1024)).toFixed(2)} MB`;
  }

  function fmtDate(v) {
    if (!v) return "-";
    const d = new Date(v);
    if (Number.isNaN(d.getTime())) return v;
    return d.toLocaleString();
  }

  function buildActionButton(label, cls, onClick) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = label;
    if (cls) btn.className = cls;
    btn.addEventListener("click", onClick);
    return btn;
  }

  function renderFiles(files) {
    filesBody.innerHTML = "";
    if (!files.length) {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td colspan="6">暂无文件</td>`;
      filesBody.appendChild(tr);
      return;
    }

    files.forEach((file) => {
      const tr = document.createElement("tr");
      const transcodeEnabled = !!file.is_text;

      const nameCell = document.createElement("td");
      nameCell.textContent = file.name;
      tr.appendChild(nameCell);

      const timeCell = document.createElement("td");
      timeCell.textContent = fmtDate(file.created_at);
      tr.appendChild(timeCell);

      const sizeCell = document.createElement("td");
      sizeCell.textContent = sizeText(file.size_bytes);
      tr.appendChild(sizeCell);

      const encCell = document.createElement("td");
      encCell.textContent = file.encoding || "Unknown";
      tr.appendChild(encCell);

      const textCell = document.createElement("td");
      textCell.textContent = file.is_text ? "是" : "否";
      tr.appendChild(textCell);

      const actionsCell = document.createElement("td");
      const actions = document.createElement("div");
      actions.className = "actions";

      actions.appendChild(buildActionButton("下载", "", async () => {
        try {
          const data = await requestJSON(`/api/files/${encodeURIComponent(file.id)}/download-token`, { method: "POST" });
          window.location.href = data.url;
        } catch (err) {
          setMsg(listMsg, `下载失败: ${err.message}`);
        }
      }));

      actions.appendChild(buildActionButton("重命名", "alt", async () => {
        const next = window.prompt("输入新文件名", file.name);
        if (!next || next === file.name) return;
        try {
          await requestJSON(`/api/files/${encodeURIComponent(file.id)}`, {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ name: next }),
          });
          await loadFiles();
          setMsg(listMsg, "重命名成功");
        } catch (err) {
          setMsg(listMsg, `重命名失败: ${err.message}`);
        }
      }));

      actions.appendChild(buildActionButton("删除", "danger", async () => {
        if (!window.confirm(`确认删除 ${file.name} ?`)) return;
        try {
          const res = await fetch(`/api/files/${encodeURIComponent(file.id)}`, { method: "DELETE" });
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          await loadFiles();
          setMsg(listMsg, "删除成功");
        } catch (err) {
          setMsg(listMsg, `删除失败: ${err.message}`);
        }
      }));

      const transcodeBtn = buildActionButton("转码", "alt", async () => {
        const source = (window.prompt("sourceEncoding (auto/具体编码)", "auto") || "auto").trim();
        const target = (window.prompt(`targetEncoding: ${ENCODINGS.join("/")}`, "UTF-8") || "").trim();
        if (!target) return;
        if (!ENCODINGS.includes(target)) {
          setMsg(listMsg, "转码失败: 目标编码不在允许列表");
          return;
        }
        try {
          await requestJSON(`/api/files/${encodeURIComponent(file.id)}/transcode`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ sourceEncoding: source, targetEncoding: target }),
          });
          await loadFiles();
          setMsg(listMsg, "转码成功");
        } catch (err) {
          setMsg(listMsg, `转码失败: ${err.message}`);
        }
      });
      transcodeBtn.disabled = !transcodeEnabled;
      actions.appendChild(transcodeBtn);

      actions.appendChild(buildActionButton("设为下载二维码目标", "alt", () => {
        selectedFileIdForBridgeDownload = file.id;
        setMsg(qrMsg, `已选择: ${file.name}`);
      }));

      actionsCell.appendChild(actions);
      tr.appendChild(actionsCell);
      filesBody.appendChild(tr);
    });
  }

  async function loadFiles() {
    setMsg(listMsg, "加载中...");
    try {
      const files = await requestJSON("/api/files");
      renderFiles(Array.isArray(files) ? files : []);
      setMsg(listMsg, "");
    } catch (err) {
      setMsg(listMsg, `加载失败: ${err.message}`);
    }
  }

  function renderQR(resp) {
    qrBox.classList.remove("hidden");
    qrImg.src = resp.qrUrl;
    qrLink.href = resp.pageUrl;
    qrLink.textContent = new URL(resp.pageUrl, window.location.origin).toString();
  }

  uploadForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    setMsg(uploadMsg, "上传中...");
    try {
      const fd = new FormData(uploadForm);
      await requestJSON("/api/files", { method: "POST", body: fd });
      uploadForm.reset();
      await loadFiles();
      setMsg(uploadMsg, "上传成功");
    } catch (err) {
      setMsg(uploadMsg, `上传失败: ${err.message}`);
    }
  });

  refreshBtn.addEventListener("click", loadFiles);

  bridgeUploadBtn.addEventListener("click", async () => {
    setMsg(qrMsg, "生成中...");
    try {
      const data = await requestJSON("/api/bridge/upload", { method: "POST" });
      renderQR(data);
      setMsg(qrMsg, "手机扫码上传");
    } catch (err) {
      setMsg(qrMsg, `生成失败: ${err.message}`);
    }
  });

  bridgeDownloadBtn.addEventListener("click", async () => {
    if (!selectedFileIdForBridgeDownload) {
      setMsg(qrMsg, "请先在文件列表选择一个目标文件");
      return;
    }
    setMsg(qrMsg, "生成中...");
    try {
      const data = await requestJSON("/api/bridge/download", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ fileId: selectedFileIdForBridgeDownload }),
      });
      renderQR(data);
      setMsg(qrMsg, "手机扫码下载");
    } catch (err) {
      setMsg(qrMsg, `生成失败: ${err.message}`);
    }
  });

  loadFiles();
})();

