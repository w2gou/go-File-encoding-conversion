# 前置说明
本文档基于 `coding/design.md`，把“要做什么”拆成“怎么一步一步做”，目标是你照着做就能把功能从 0 落地到可用版本（LAN 内存中转 + 文本转码 + 静态前端 + 一次性链接/二维码）。

约束复述（确保你在实现过程中不跑偏）：
- LAN 场景；无登录；现阶段只用内存存储（重启即丢）。
- 单文件 ≤ 100MB；文件数 ≤ 10000；总内存默认 ≤ 300MB（硬保护）；超限上传触发 FIFO 删除最老文件。
- 上传若缺失 `Content-Length`：直接拒绝（更安全）。
- 文件名全局唯一（区分大小写）；重名上传/重命名冲突：直接拒绝并保持原名不变。
- 仅对“可识别文本”开放转码；判定要保守（宁可少放行）；转码严格失败且不覆盖原文件。
- 下载/二维码均需“一次性使用”（one-time token）。
- 前端：纯静态 HTML/JS（不引入框架）。
- 配置文件：YAML；默认监听 `0.0.0.0:8080`；`server.base_url` 填手机可达的 LAN IP origin（如 `http://192.168.1.10`）。

说明：
- 本文档不依赖外网资料；涉及第三方库仅给出推荐项，你也可以替换，但要保证行为等价（尤其是“严格失败/一次性 token/限流/硬保护”）。

---

# Sequential‑Thinking 分析
1. 先把“配置 + 受控内存仓库 + token 仓库”做成可测试的纯 Go 组件，保证硬约束正确（否则后面 HTTP/前端只是把错暴露出来）。
2. 再实现 HTTP API（先 PC 管理页，再手机桥接），每加一个 API 都配一条最小可验证的 `curl`/浏览器操作。
3. 最后补齐“编码识别/文本判定/转码”与并发保护（上传/转码信号量、超时、大小限制、Content-Length 拒绝），以免在高并发目标下 OOM 或卡死。

---

# 0. 开发准备
## 0.1 环境要求
- Go：以 `go.mod` 为准（当前写的是 `go 1.24.0`）。如果你本机 Go 版本不支持该版本号，先把 `go.mod` 里的 `go 1.24.0` 改成你本机版本（例如 `go 1.22.0`），否则无法 `go test/go build`。
- 建议：Linux/WSL 开发更方便；Windows 也可。

## 0.2 推荐目录结构（你可以按需微调）
建议把主程序薄化，把可复用逻辑放到 `internal/`：
- `main.go`：读配置、组装依赖、启动 HTTP server
- `internal/config/`：YAML 配置结构、默认值、校验、URL 生成
- `internal/store/`：内存文件仓库（FIFO、索引、总量统计、并发安全）
- `internal/tokens/`：一次性 token（下载/桥接）仓库（TTL、原子消费、清理）
- `internal/text/`：文本判定、编码探测、转码实现
- `internal/httpapi/`：路由与 handler（API + 静态页面）
- `web/`：静态资源（PC 页、手机页、JS、CSS）

---

# 1. 第一步：把配置做对（YAML + 校验 + URL 口径）
目标产物：
- 能用 `-config ./config.yaml` 启动；
- 默认监听 `0.0.0.0:8080`；
- `server.base_url` 按设计口径（手机可达 origin，不含端口也可）生成对外 URL。

建议步骤：
1. 定义配置结构体（对应 `coding/design.md` 的示例 YAML）：`server.listen`、`server.base_url`、timeouts、limits、tokens。
2. 实现 `Load(path) (Config, error)`：
   - 文件不存在/解析失败：返回明确错误。
   - 允许缺省字段：填充默认值。
3. 实现 `Validate() error`：
   - `server.listen` 必须含端口；
   - 限额必须为正；
   - `max_file_size_mb <= 100`（按需求固化）；
   - `max_total_size_mb` 默认 300（可改）。
4. 实现 `ExternalOrigin()`（用于拼下载/二维码 URL）：
   - `server.base_url` 若显式含端口：直接用；
   - 若不含端口：从 `server.listen` 取端口，端口为 80 则不拼，否则拼 `:port`。

最小自检：
- `go run . -config ./config.yaml` 能启动；
- 打印一行日志：listen/base_url/origin/限额（便于排错）。

---

# 2. 第二步：实现 In‑Memory File Store（硬约束优先）
目标产物：
- 在纯 Go 层面实现：Add/Get/List/Delete/Rename/ReplaceBytes（转码覆盖）；
- 保证：文件名全局唯一（区分大小写）；重名冲突拒绝且不改变原状态；
- 保证：FIFO 淘汰最老文件；count/bytes 双约束；线程安全。

建议数据结构（按 `coding/design.md`）：
- `byID map[string]*entry`
- `byName map[string]string`（name -> id）
- `fifo list.List`（队头最老）
- `totalBytes int64`
- `mu sync.RWMutex`
- （可选）每个文件一个 `sync.Mutex` 或在 store 层做“对单文件写操作串行化”（转码/删除/重命名冲突时好处理）

关键实现要点：
1. Add（上传落库）：
   - 参数包含：原始文件名、bytes、探测出的 `encoding/isText`、时间戳。
   - 先检查重名（`byName`），冲突返回固定错误（用于 HTTP 409 “重名”）。
   - 再检查“单文件 > max_total_bytes”直接拒绝（否则永远塞不进去）。
   - 再做 FIFO 淘汰：循环踢出队头直到满足 `max_files/max_total_bytes` 或无可淘汰。
   - 仍不满足：拒绝（用于 HTTP 507/413）。
2. Rename：
   - 新名冲突：返回“重名”错误；不修改任何索引与 `FileItem.name`（等价于“还原”）。
3. Delete：
   - 必须从 `byID/byName/fifo/totalBytes` 同步移除；保证幂等或给出明确 404。
4. ReplaceBytes（转码成功后覆盖）：
   - 只在转码成功后调用；
   - 更新 `bytes/sizeBytes/encoding/isText`（isText 可保持 true），并更新 `totalBytes`；
   - 若导致超过 `max_total_bytes`：建议拒绝覆盖（严格失败）或先淘汰其它文件后再覆盖；优先“严格失败”（更可控）。

最小自检（强烈建议先写单元测试）：
- 重名 Add 拒绝；
- Rename 冲突拒绝且原名未变；
- FIFO：连续 Add 超过上限时最老被删除；
- totalBytes：边界值（恰好等于上限/刚超过上限）行为正确；
- 并发：`go test -race` 不报 data race（后续再跑也行）。

---

# 3. 第三步：实现一次性 Token Store（下载/桥接通用）
目标产物：
- 生成 token：随机、不可预测；
- TTL 过期自动失效；
- “一次性使用”：首次消费成功，后续访问必失败（410/404）。

建议实现：
1. token 结构：
   - `value string`
   - `kind string`（download/bridge-upload/bridge-download 等）
   - `payload`（如 `fileID`）
   - `expiresAt time.Time`
2. store 结构：
   - `mu sync.Mutex`
   - `items map[string]tokenItem`
   - 后台清理：`time.Ticker` 每 N 秒扫一次（或惰性清理：每次 Get/Consume 时顺便清理过期）。
3. 原子消费 `Consume(token) (payload, ok)`：
   - 先检查存在且未过期；
   - 立刻从 map 删除；
   - 返回 payload。

最小自检：
- 同一 token 连续 Consume：第一次成功，第二次失败；
- 过期 token：失败；
- 并发 Consume：只能有一个成功（用 `-race` 验证）。

---

# 4. 第四步：实现“可识别文本”判定 + 编码探测 + 严格转码
目标产物：
- 上传后能得到 `isText` 与 `encoding`（Unknown/UTF-8/GB18030/GBK/Big5/Windows-1252/ISO-8859-1）；
- 仅 `isText=true` 才允许转码；
- 转码严格失败：失败不覆盖原 bytes。

建议实现顺序：
1. 定义候选编码与下拉框顺序（必须固定）：
   1) UTF-8  2) GB18030  3) GBK  4) Big5  5) Windows-1252  6) ISO-8859-1
2. 实现二进制快速排除（保守）：
   - 取样前 64KB（或全量若更小）；
   - 若 NUL（0x00）出现，或控制字符占比超阈值，判 `isText=false`。
3. UTF-8 判定：
   - `utf8.Valid(sample)` 且可打印字符占比达标 → `isText=true, encoding=UTF-8`。
4. 非 UTF-8 探测：
   - 依序尝试 strict decode（用 `x/text/encoding` + `transform`）；
   - strict decode 成功且可打印字符占比达标 → `isText=true, encoding=命中项`；
   - 全部失败 → `isText=false, encoding=Unknown`。
5. 严格转码：
   - 入参：`sourceEncoding`（auto 或具体编码）、`targetEncoding`（下拉框中的一个）；
   - auto：用已探测 encoding（Unknown 则拒绝）；
   - decode/encode 任一步失败：返回错误；不覆盖原 bytes；
   - 成功：返回新 bytes + 新 encoding。

并发保护：
- 转码处理必须有 `transcodeSemaphore`（默认 2，可配置），否则在 1000 并发下容易被转码 CPU/内存拖垮。

最小自检：
- 给一段 UTF-8 文本：识别为 UTF-8；
- 给一段 GBK/GB18030 文本：能命中（或至少在手工指定 sourceEncoding 时能转码成功）；
- 给一个二进制文件（含大量 0x00）：必须判 `isText=false`；
- 转码失败不改变 store 中原 bytes（需要 store 层测试或 handler 层集成测试）。

---

# 5. 第五步：搭 HTTP Server 骨架（先跑通，再加功能）
目标产物：
- 能启动并提供静态页（`/`、`/m/upload/{token}`、`/m/download/{token}`）；
- API 有统一错误 JSON；
- 有基础超时（ReadHeader/Read/Write/Idle）。

路由建议（两种选其一）：
- A：标准库 `net/http` + 自己解析 path（依赖少，但写 path 参数麻烦）；
- B：使用轻量 router（例如 chi），实现更快更稳（建议）。

无论选哪种，都先实现这些通用件：
1. `JSON(w, status, v)` 与 `Error(w, status, code, message, detail)`。
2. request 日志：method/path/status/耗时（最低限度即可）。
3. server 超时：按配置设置到 `http.Server`。

最小自检：
- 打开浏览器访问 `/` 能返回页面（哪怕只是 “OK” 占位页）。

---

# 6. 第六步：实现 API（按“最小闭环”逐个上线）
建议按下面顺序做，每个接口做完立即手动验证：

## 6.1 GET /api/files（列表）
- 返回：文件列表（不含 bytes），含 `id/name/size/createdAt/encoding/isText`。
- 验证：上传前为空；上传后可见。

## 6.2 POST /api/files（上传，PC/手机共用）
必须同时做的安全点：
- 缺失 `Content-Length` → 411；
- `MaxBytesReader` → 限 100MB + 少量开销；
- `uploadSemaphore` → 满了就 503 + `Retry-After`；
- 重名 → 409（“重名”）；
- 淘汰后仍放不下 → 507/413（可读提示）。

验证：
- 正常上传小文本；
- 同名上传拒绝；
- 上传大于 100MB（或用配置调小便于测试）必须拒绝；
- 连续上传直到触发 FIFO（用小上限更好测）。

## 6.3 DELETE /api/files/{id}
- 验证：删除后列表消失；下载/转码应返回 404。

## 6.4 PATCH /api/files/{id}（重命名）
- 冲突：409（“重名”），并保持原名不变。
- 验证：把 A 改成已存在的 B，应该失败且 A 仍叫 A。

---

# 7. 第七步：实现一次性下载（Download Token + /dl/{token}）
目标产物：
- 点击下载/请求 token → 跳转 `/dl/{token}`；
- token 一次性：第二次访问必失败（410/404）；
- 浏览器默认下载行为即可（不实现“选择目录”）。

建议实现：
1. `POST /api/files/{id}/download-token`：
   - 返回 `{token,url}`（url 形如 `/dl/{token}`）。
2. `GET /dl/{token}`：
   - 原子消费 token；
   - 找到文件 bytes；
   - 设置 header：
     - `Content-Disposition: attachment; filename="..."`（注意引号与必要的转义）
     - `Content-Type: application/octet-stream`
   - 用 `http.ServeContent`（推荐）或 `io.Copy` 输出。

验证：
- 生成一次下载链接后下载成功；
- 再访问同一链接失败；
- 文件名中含空格/中文时下载文件名显示正常（至少在 Chrome 下）。

---

# 8. 第八步：实现二维码桥接（一次性 bridge token）
目标产物：
- PC 点“手机上传/手机下载”生成二维码；
- 手机扫码打开页面（微信扫码跳浏览器）；
- bridge token 一次性：同一二维码第二次使用失败。

建议实现（按 `coding/design.md` API 口径）：
1. `POST /api/bridge/upload`：创建 `bridgeToken(kind=upload)`，返回：
   - `pageUrl`：`/m/upload/{bridgeToken}`（给前端转成绝对 URL 用）
   - `qrUrl`：`/qrcode/{bridgeToken}.png`
2. `GET /m/upload/{bridgeToken}`：手机上传页（静态 HTML）。
3. `POST /api/bridge/{bridgeToken}/upload`：消费 token 并写入 store（同 `/api/files` 的校验要复用）。
4. `POST /api/bridge/download`：创建 `bridgeToken(kind=download, payload=fileID)`，返回 `pageUrl/qrUrl`。
5. `GET /m/download/{bridgeToken}`：手机下载页（显示文件信息 + 下载按钮）。
6. `POST /api/bridge/{bridgeToken}/download-token`：
   - 消费 bridge token（一次性）；
   - 创建 download token；
   - 返回 `/dl/{token}` 或直接 302 跳转（两种都行，建议返回 JSON 由前端跳）。
7. `GET /qrcode/{bridgeToken}.png`：
   - 生成二维码内容：手机可达的绝对 URL（用 `ExternalOrigin()+pageUrl`）；
   - 输出 PNG。

验证：
- PC 生成二维码；
- 手机扫开页面能上传/下载；
- 二维码重复扫：必须失败（或提示“已使用/已过期”）。

---

# 9. 第九步：补齐 PC/手机静态前端（纯 HTML/JS）
目标产物：
- `/`：PC 管理页（上传、列表、重命名、删除、下载、转码、生成二维码）
- `/m/upload/{token}`：手机上传页
- `/m/download/{token}`：手机下载页

建议实现要点（避免引入框架）：
1. PC 页：
   - 首屏拉取 `/api/files` 渲染表格；
   - 上传用 `<input type="file">` + `fetch` 走 multipart；
   - 下载按钮：先 `POST /api/files/{id}/download-token`，再把浏览器跳转到 `url`；
   - 重命名：弹窗输入新名，调用 `PATCH`，处理 409 “重名”提示；
   - 转码：若 `isText=false` 则禁用；否则弹窗选择 source（auto/手动）与 target（固定顺序下拉），调用 `/api/files/{id}/transcode`；
   - 手机上传/手机下载：点击后调用 `/api/bridge/...` 拿到 `qrUrl`，展示二维码图片与说明。
2. 手机页：
   - 上传页：只做上传 + 成功/失败提示；
   - 下载页：展示文件名/大小/编码（可选），下载按钮触发创建下载 token 后跳转 `/dl/...`。
3. 静态资源加载：
   - 推荐用 Go `embed` 把 `web/` 打进二进制，避免部署时丢文件。

验证：
- 只用浏览器就能完成“上传→列表→下载→重命名→删除→二维码手机上传/下载→转码”完整闭环。

---

# 10. 第十步：实现转码 API（严格失败 + 并发保护 + 文件级写保护）
目标产物：
- `POST /api/files/{id}/transcode` 完全符合设计口径；
- 并发下不出现：转码与删除/再次转码互相踩踏导致的数据竞争或部分写入。

建议实现：
1. Handler 入口校验：
   - 文件不存在：404；
   - `isText=false`：400（明确提示“不支持转码”）；
   - target 不在允许列表：400；
2. 获取 `transcodeSemaphore`（满了就 503 + Retry-After）。
3. 文件级互斥：
   - 要么在 store 里提供 `WithFileWriteLock(id, fn)`；
   - 要么 store 的 ReplaceBytes/Rename/Delete 全走同一把 per-file lock。
4. 执行严格转码：
   - 失败：返回错误，不修改 bytes；
   - 成功：调用 store.ReplaceBytes 更新 bytes/encoding/sizeBytes/totalBytes。

验证：
- 转码失败后文件仍可正常下载且内容未变；
- 转码成功后 encoding 更新；
- 并发点转码/删除不会崩溃（至少不 data race）。

---

# 11. 第十一步：把“硬保护与高并发可用”落到代码里
这是保证“1000 并发下不崩溃”的关键步骤，建议逐条打勾实现：
- [ ] 上传缺失 `Content-Length`：411（不进入读取流程）
- [ ] `MaxBytesReader`：硬限 100MB
- [ ] 上传并发信号量：默认 16（可配），满了快速拒绝 503
- [ ] 转码并发信号量：默认 2（可配），满了快速拒绝 503
- [ ] store 层双上限：max_files=10000 + max_total_size_mb=300（可配）
- [ ] 上传/覆盖前后都校验 totalBytes（避免瞬时突破）
- [ ] HTTP 超时：ReadHeader/Read/Write/Idle（可配）
- [ ] 下载慢连接不会无限占用（WriteTimeout 生效）

验证建议（不追求“都成功”，追求“不崩溃、错误可解释”）：
- 用浏览器多开标签页同时上传/下载，看是否出现 OOM/卡死；
- 观察日志：是否能看到 503/411/409/507 的合理原因。

---

# 12. 第十二步：验证清单（按验收标准逐项过）
你可以按下面顺序做一次“验收回归”：
1. 启动：`./app -config ./config.yaml`
2. PC 上传小文本：成功；列表出现；encoding/isText 合理。
3. 重名上传：409 “重名”。
4. 重命名冲突：409 “重名”，原名不变。
5. 删除：列表消失；下载/转码 404。
6. 下载一次性：同一链接重复下载失败。
7. 二维码一次性：同一二维码重复扫码失败。
8. 触发 FIFO：把 `max_total_size_mb` 临时调小（如 5），连续上传触发淘汰最老文件。
9. 缺失 `Content-Length`：构造 chunked 上传（可选工具）应返回 411。
10. 转码：仅 `isText=true` 文件可转码；失败不覆盖；成功更新 encoding。

可选：并发压测（LAN 内自用即可，别把网打爆）：
- 下载：多并发拉 `/dl/...`（注意 token 一次性，先批量生成 token）
- 上传：多并发 `POST /api/files`，观察是否触发 503 而不是 OOM

---

# 13. 第十三步：打包与运行（Windows/Linux/Docker）
## 13.1 本机构建
- Linux：`GOOS=linux GOARCH=amd64 go build -o app .`
- Windows：`GOOS=windows GOARCH=amd64 go build -o app.exe .`

## 13.2 运行
- `./app -config ./config.yaml`

## 13.3 LAN 使用提醒
- 确保手机能访问 `server.base_url` 对应的机器（同网段、无防火墙拦截端口）。
- 若 `server.base_url` 不含端口且 listen 不是 80，二维码/外链会自动拼端口（按设计口径）。

---

# 需要你最终确认/补充（避免实现时返工）
1. 你希望 `/api/files` 列表里是否要暴露“原始 MIME/是否二进制”的额外字段？（不必做也能用）
2. 二维码 token/下载 token 的 TTL 是否保持设计默认：下载 60s、桥接 300s？（可配，但默认值要定）
3. 转码 API 是否允许“转码成功后自动重命名文件”（例如追加 `.utf8.txt`）？当前设计是覆盖原文件且文件名不变。
