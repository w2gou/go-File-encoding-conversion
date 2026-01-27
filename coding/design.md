# 前置说明
本文档是基于 `coding/requirement.md` 与你最新补充信息生成的开发方案（面向落地实现），用于指导本仓库的 Go Web 服务开发、交付与验收。

已确认并固化的关键口径（最终以此为准）：
1. PC/手机“选择下载目录”接受用浏览器默认下载行为替代（可配合浏览器“每次询问保存位置”）。
2. 使用场景：局域网内使用（LAN）。
3. 文件类型与大小：允许任意文件上传；但仅对“可识别文本”开放转码；单文件最大 100MB。
4. 规模：最多保存 10000 个文件；超过后在上传时自动删除最老上传文件（FIFO）。
5. 总内存上限：`MAX_TOTAL_SIZE_MB` 默认 300MB（硬保护）。
6. 转码失败策略：严格失败并提示（失败不覆盖原文件，不做替换/容错写回）。
7. 安全策略：一次性下载链接 + 二维码一次性使用。
8. 前端形态：纯静态 HTML/JS。
9. 并发目标：同时在线约 1000；并发上传 1000；并发下载 1000（允许限流/拒绝，但不崩溃）。
10. 重命名规则：不允许重名；冲突时直接拒绝（返回“重名”），文件名保持原值不变。
11. 文件名唯一性：区分大小写（`a.txt` 与 `A.txt` 不算重名）。
12. 手机扫码：微信/浏览器；微信扫码会跳转到浏览器。
13. 目标编码下拉框：按本文建议列表与顺序。
14. “可识别文本”判定：宁可少放行（更安全，避免误把二进制当文本转码）。
15. LAN 访问地址与端口等：使用配置文件配置。
16. 配置文件格式：选用 YAML。
17. 默认监听：`0.0.0.0:8080`。
18. `server.base_url`：填写“手机可达的 LAN IP”（例如 `http://192.168.1.10`）。
19. 上传请求若缺失 `Content-Length`：直接拒绝（更安全，避免内存不可控）。
20. 现阶段仅内存存储自用；未来可演进到落盘/数据库/对象存储。

重要提醒（正确性优先）：
- “100MB/文件 + 10000 文件”理论上可达 1TB，而默认总内存仅 300MB，因此系统在默认配置下会更倾向“少量文件常驻 + 大文件快速触发 FIFO 淘汰”；这符合你“只放内存、自己用”的现阶段定位。
- 在“并发上传 1000”目标下，必须实现上传并发/内存的硬限流与提前拒绝，否则极易 OOM 或导致 GC 抖动，进而影响下载与 UI。

---

# Sequential‑Thinking 分析
1. 需求本质：LAN 场景的轻量文件中转与文本编码转换服务（无 DB、内存仓库、Web UI、二维码桥接手机）。
2. 可行性关键：已接受浏览器默认下载行为，因此无需实现“选择目录写入”类能力，跨浏览器稳定性更高。
3. 最大工程风险：默认 300MB 总内存与 100MB 单文件上限叠加高并发上传目标，必须“限流 + 准入控制 + 淘汰/拒绝”。
4. 安全边界：LAN 无登录也需防链接扩散；一次性 token（下载/二维码）+ 短 TTL + 原子消费是核心。
5. base_url 口径：你希望只填“手机可达的 LAN IP（不含端口）”，因此需要在服务端生成链接时把监听端口合并进最终 URL（除非端口为 80 或 base_url 显式包含端口）。

---

# 1. 目标与范围
## 1.1 Goals
- Web UI：上传、列表、重命名、删除、下载、转码覆盖。
- 手机扫码：手机端上传/下载（一次性二维码页面）。
- 编码探测与展示；支持选择目标编码转码并覆盖原文件。
- 高并发下稳定：峰值请求下可限流/拒绝，但服务不崩溃、数据结构不损坏、错误可解释。
- 交付：Windows/Linux 可执行文件 + Docker 构建文件。

## 1.2 Non‑Goals（本期不做/不保证）
- 用户系统/权限/审计。
- 持久化保证（重启即丢）。
- 100% 准确编码探测（提供手动指定源编码纠错入口）。

---

# 2. 非功能约束与关键指标
## 2.1 资源保护（必须实现）
- 单文件上限：100MB（硬拒绝）。
- 文件数上限：10000（上传触发 FIFO 淘汰最老文件）。
- 总内存上限：300MB（默认，硬保护；上传触发 FIFO 淘汰，仍不满足则拒绝）。
- 上传并发限制：必须（建议默认 16，可配置）。
- 转码并发限制：必须（建议默认 2，可配置）。
- token TTL：下载 token 默认 60s；二维码 token 默认 300s（可配置）。

## 2.2 “并发 1000”目标的实现口径
- 下载并发 1000：以网络 IO 为主，可支持较高并发，但需合理超时与避免慢连接长期占用。
- 上传并发 1000：在 300MB 总内存约束下不可能“都成功”；应通过限流/准入控制快速拒绝，保证整体可用。
- UI 与二维码页面：优先保证响应，不被大文件上传/转码拖垮。

---

# 3. 总体架构
## 3.1 组件
- Go HTTP Server：API + 静态资源（前端 HTML/JS/CSS）。
- In‑Memory File Store：文件 bytes + 元数据；FIFO 队列；总量统计（count/bytes）；并发安全；name 唯一索引（区分大小写）。
- Token Store：一次性 token（下载 token、二维码会话 token），TTL 清理、原子消费。
- Encoding Service：编码探测 + 严格转码；严格“可识别文本”判定。
- QR 生成：二维码 PNG（或 SVG）输出。

## 3.2 最终对外 URL 生成规则（用于二维码/跳转链接）
配置：
- `server.listen`：监听地址（默认 `0.0.0.0:8080`）
- `server.base_url`：手机可达 LAN IP（例如 `http://192.168.1.10`，不含端口）

规则（建议固化为实现逻辑）：
1. 解析 `server.listen` 得到端口 `listenPort`（默认 8080）。
2. 若 `server.base_url` 已包含端口，则直接使用该 origin。
3. 若 `server.base_url` 不含端口：
   - 若 `listenPort` 为 80：最终 origin = `server.base_url`
   - 否则最终 origin = `server.base_url + ":" + listenPort`

示例：
- base_url=`http://192.168.1.10`，listen=`0.0.0.0:8080` → 最终 origin=`http://192.168.1.10:8080`

---

# 4. 内存数据模型
## 4.1 FileItem（核心对象）
- `id`：服务端生成唯一 ID（避免用文件名当主键）。
- `name`：文件名（全局唯一，区分大小写）。
- `createdAt`：上传时间（FIFO 排序依据）。
- `sizeBytes`：字节数。
- `encoding`：当前编码（探测/确认值）。
- `isText`：是否“可识别文本”（决定是否开放转码）。
- `bytes`：文件内容（`[]byte`）。

## 4.2 索引与淘汰结构（O(1)）
- `byID map[string]*entry`
- `byName map[string]string`（name → id，用于全局唯一，区分大小写）
- `fifo list.List`（按上传顺序，队头最老）
- `totalBytes int64`

---

# 5. 核心流程设计
## 5.1 上传（PC 与手机共用）
关键点：在读取大文件到内存前尽量做“准入控制”，减少瞬时内存尖峰。

流程：
1. 接收 multipart 文件流。
2. 校验 `Content-Length`：
   - 若缺失：直接拒绝（`411 Length Required`），返回可读提示（“为保证内存可控，需要 Content-Length”）。
3. 使用 `MaxBytesReader` 强制限制请求体最大字节数（100MB + 少量开销）。
4. 获取上传并发信号量；若已满则快速拒绝（503，带 `Retry-After`）。
5. name 唯一校验：
   - 若上传文件名已存在：直接拒绝（409 Conflict，“重名”）。
6. 触发 FIFO 淘汰（按文件数/总内存）：
   - 淘汰后仍无法容纳本次上传（例如单文件 > 总上限）：拒绝上传（507/413，说明原因）。
7. 读取到内存 buffer，写入 store。
8. 编码探测 + “可识别文本”判定，写入元数据。

## 5.2 FIFO 淘汰（上传触发）
触发条件：`count > MAX_FILES` 或 `totalBytes > MAX_TOTAL_BYTES`。
策略：循环淘汰 `fifo` 队头（最老上传）直到满足限制或无可淘汰。

## 5.3 下载（一次性链接）
流程：
1. 前端点击下载 → 请求创建 `DownloadToken(fileId, ttl=60s, one-time)`。
2. 前端跳转 `/dl/{token}`，由浏览器处理保存路径。
3. 服务端在下载请求到达时原子消费 token；重复访问返回 410/404。

## 5.4 手机扫码上传/下载（二维码一次性）
- PC 点击“手机上传/手机下载”创建 `BridgeToken(ttl=300s, one-time)` 并展示二维码。
- 手机扫码打开 `/m/.../{bridgeToken}` 页面：
  - 上传：提交到 `/api/bridge/{bridgeToken}/upload`，消费 token。
  - 下载：点击下载时创建一次性 `DownloadToken` 并重定向 `/dl/{token}`；`BridgeToken` 同时失效（一次性）。

## 5.5 重命名（不允许重名，冲突不修改）
流程：
1. `PATCH /api/files/{id}` 传入新 `name`。
2. 若 `name` 已被其它文件占用：返回 409（“重名”），文件名不变（等价于“还原/不修改”）。
3. 成功则更新 `byName` 索引与 `FileItem.name`。

## 5.6 转码（仅文本、严格失败）
入口控制：
- `isText=false`：UI 隐藏转码或提示“不支持转码（非可识别文本）”。
- `isText=true`：允许转码；失败严格提示且不覆盖原 bytes；成功才覆盖并更新编码。

---

# 6. API 设计（建议）
统一错误结构（示例）：`{ "code":"...", "message":"...", "detail":"..." }`。

## 6.1 文件管理
- `GET /api/files`：文件列表（不含 bytes）。
- `POST /api/files`：上传（`multipart/form-data`，字段 `file`）。
- `PATCH /api/files/{id}`：重命名（`{"name":"..."}`；全局唯一，区分大小写）。
- `DELETE /api/files/{id}`：删除。

## 6.2 下载（一次性）
- `POST /api/files/{id}/download-token` → `{"token":"...","url":"/dl/{token}"}`
- `GET /dl/{token}`：消费 token 并下发 `Content-Disposition: attachment`

## 6.3 转码
- `POST /api/files/{id}/transcode`
  - 请求：`{"targetEncoding":"UTF-8","sourceEncoding":"auto|GB18030|GBK|Big5|Windows-1252|ISO-8859-1"}`
  - 规则：仅 `isText=true`；严格失败；成功才覆盖 bytes 与更新 `encoding`

## 6.4 二维码桥接
- `POST /api/bridge/upload` → `{bridgeToken,pageUrl,qrUrl}`
- `POST /api/bridge/download`（入参 `{fileId}`）→ `{bridgeToken,pageUrl,qrUrl}`
- `POST /api/bridge/{bridgeToken}/upload`：手机上传提交
- `POST /api/bridge/{bridgeToken}/download-token`：手机点击下载触发一次性下载 token
- `GET /m/upload/{bridgeToken}`、`GET /m/download/{bridgeToken}`：手机静态页面
- `GET /qrcode/{bridgeToken}.png`：二维码图片

---

# 7. “可识别文本”判定与编码方案
## 7.1 目标编码下拉框（最终清单与顺序）
1. UTF-8
2. GB18030
3. GBK
4. Big5
5. Windows-1252
6. ISO-8859-1

## 7.2 文本判定（宁可少放行）
目标：避免把二进制误判为文本，从而开放转码导致内容破坏。

建议实现为“强条件判定”（偏保守）：
1. 二进制快速排除：在前 64KB（或文件全量若更小）中，若出现较多 `0x00`（NUL）或不可打印控制字符占比超过阈值 → `isText=false`。
2. UTF-8 判定：`utf8.Valid(bytes)` 且可打印字符占比达标 → `isText=true`，encoding=UTF-8。
3. 非 UTF-8：按候选编码列表（GB18030、GBK、Big5、Windows-1252、ISO-8859-1）逐个做严格解码：
   - 严格解码成功且可打印字符占比达标 → `isText=true`，encoding=该编码；
   - 否则 `isText=false`，encoding=Unknown。

说明：保守策略会让一部分“边界文本”（混合二进制/控制字符较多）被判定为非文本，转码入口会被禁用，这是符合你“更安全”的偏好。

## 7.3 严格转码
严格失败意味着：
- 解码失败/编码失败立即返回错误；
- 原文件 bytes 不修改；
- 成功时原子性覆盖 bytes 并更新 `encoding`。

---

# 8. 并发与稳定性设计
## 8.1 上传：限流 + 准入控制 + 提前拒绝
- `uploadSemaphore`：限制同时处理上传的数量（默认建议 16，可配置）。
- 强制大小限制：`MaxBytesReader`。
- 早拒绝：
  - 缺失 `Content-Length`：411。
  - 上传并发已满：503 + `Retry-After`（例如 1~3s）。
  - 重名：409 Conflict。
  - 淘汰后仍无法满足总内存：507 Insufficient Storage（或 413），给出可读提示。

## 8.2 下载：高并发可读
- 使用 `bytes.Reader` + `io.Copy` 输出，避免多余拷贝。
- 设置 `WriteTimeout`，避免慢客户端长期占用连接。

## 8.3 转码：并发限制 + 文件级互斥
- `transcodeSemaphore`：限制并发转码数量（默认 2）。
- 单文件转码期间应避免与删除/再次转码并发冲突（加文件级锁或在 store 层序列化该文件的写操作）。

## 8.4 HTTP 超时（建议默认，可配置）
- `ReadHeaderTimeout`：5s
- `ReadTimeout`：300s
- `WriteTimeout`：300s
- `IdleTimeout`：60s

---

# 9. 前端（纯静态 HTML/JS）
## 9.1 PC 管理页（`/`）
- 文件表格：名称（唯一，区分大小写）、大小、上传时间、编码、是否可转码、操作（下载/转码/重命名/删除/手机下载二维码）。
- 上传区：PC 上传；手机上传二维码按钮。
- 转码弹窗：当前编码 + 源编码（自动/手动）+ 目标编码；显示严格失败原因。

## 9.2 手机页面
- 上传页：选择文件并上传；成功提示。
- 下载页：展示文件信息；点击下载（跳转一次性链接）。

---

# 10. 配置文件与启动方式（YAML）
## 10.1 配置文件原则
- 所有运行参数（监听地址、base_url、限额、并发数、TTL、超时）由配置文件提供。
- 启动时通过命令行参数指定配置路径（默认 `./config.yaml`）。

## 10.2 配置示例
```yaml
server:
  listen: "0.0.0.0:8080"
  base_url: "http://192.168.1.10"
  timeouts:
    read_header_seconds: 5
    read_seconds: 300
    write_seconds: 300
    idle_seconds: 60

limits:
  max_file_size_mb: 100
  max_files: 10000
  max_total_size_mb: 300
  upload_concurrency: 16
  transcode_concurrency: 2

tokens:
  download_ttl_seconds: 60
  bridge_ttl_seconds: 300
```

## 10.3 启动示例
- `./app -config ./config.yaml`

---

# 11. 打包与部署
## 11.1 跨平台构建
- Windows：`GOOS=windows GOARCH=amd64 go build -o app.exe .`
- Linux：`GOOS=linux GOARCH=amd64 go build -o app .`

## 11.2 Docker
- 多阶段构建：builder 编译、runner 运行。
- 配置文件挂载：运行时将 `config.yaml` 挂载到容器并通过 `-config` 指定。

---

# 12. 验收标准（对齐需求与补充）
- 上传：PC 上传/手机扫码上传均可；单文件超 100MB 拒绝；达到上限触发 FIFO 自动淘汰最老文件；重名上传拒绝（409）；缺失 Content-Length 拒绝（411）。
- 列表：展示名称/上传时间/编码/是否可转码；支持删除。
- 重命名：不允许重名；冲突返回 409 且文件名保持不变。
- 下载：PC 下载触发浏览器默认下载；手机下载通过二维码进入页面下载；一次性下载链接重复访问失效。
- 转码：仅对 `isText=true` 开放；严格失败不覆盖；成功覆盖并更新编码。
- 稳定性：并发 1000 请求下可出现限流/拒绝，但服务不崩溃、不出现数据结构损坏；日志可定位原因。

---

# 13. 演进路线（为未来上 DB/落盘预留）
- 维持 API 语义不变：将 `In‑Memory File Store` 替换为落盘/对象存储；元数据可迁移到数据库。
- 维持 token 模型不变：一次性下载/二维码仍由服务端签发与消费。

