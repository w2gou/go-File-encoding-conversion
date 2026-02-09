(() => {
  const msg = document.getElementById("mobile-upload-msg");
  const form = document.getElementById("mobile-upload-form");

  function bridgeTokenFromPath() {
    const m = window.location.pathname.match(/^\/m\/upload\/([^/]+)$/);
    return m ? decodeURIComponent(m[1]) : "";
  }

  function setMsg(v) {
    msg.textContent = v || "";
  }

  const token = bridgeTokenFromPath();
  if (!token) {
    setMsg("链接不合法");
    form.style.display = "none";
    return;
  }

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    setMsg("上传中...");
    const fd = new FormData(form);
    const res = await fetch(`/api/bridge/${encodeURIComponent(token)}/upload`, {
      method: "POST",
      body: fd,
    });
    const text = await res.text();
    if (!res.ok) {
      setMsg(`上传失败: ${text}`);
      return;
    }
    setMsg("上传成功，二维码已消费");
    form.style.display = "none";
  });
})();

