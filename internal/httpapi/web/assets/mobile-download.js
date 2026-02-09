(() => {
  const msg = document.getElementById("mobile-download-msg");
  const btn = document.getElementById("mobile-download-btn");
  const nameEl = document.getElementById("file-name");
  const sizeEl = document.getElementById("file-size");
  const encEl = document.getElementById("file-encoding");

  function bridgeTokenFromPath() {
    const m = window.location.pathname.match(/^\/m\/download\/([^/]+)$/);
    return m ? decodeURIComponent(m[1]) : "";
  }

  function setMsg(v) {
    msg.textContent = v || "";
  }

  function sizeText(n) {
    if (typeof n !== "number") return "-";
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / (1024 * 1024)).toFixed(2)} MB`;
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

  const token = bridgeTokenFromPath();
  if (!token) {
    btn.disabled = true;
    setMsg("链接不合法");
    return;
  }

  requestJSON(`/api/bridge/${encodeURIComponent(token)}/download-info`)
    .then((data) => {
      nameEl.textContent = data.name || "-";
      sizeEl.textContent = sizeText(data.size_bytes);
      encEl.textContent = data.encoding || "Unknown";
    })
    .catch((err) => {
      btn.disabled = true;
      setMsg(`加载失败: ${err.message}`);
    });

  btn.addEventListener("click", async () => {
    btn.disabled = true;
    setMsg("准备下载...");
    try {
      const data = await requestJSON(`/api/bridge/${encodeURIComponent(token)}/download-token`, { method: "POST" });
      window.location.href = data.url;
      setMsg("已开始下载");
    } catch (err) {
      btn.disabled = false;
      setMsg(`下载失败: ${err.message}`);
    }
  });
})();

