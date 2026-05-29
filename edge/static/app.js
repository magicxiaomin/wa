const maxRecipients = 3;
const tokenInput = document.querySelector("#token");
const loadButton = document.querySelector("#load");
const sendButton = document.querySelector("#send");
const contactsNode = document.querySelector("#contacts");
const textInput = document.querySelector("#text");
const logNode = document.querySelector("#log");

let contacts = [];
const selected = new Set();

loadButton.addEventListener("click", loadContacts);
sendButton.addEventListener("click", sendMessage);

function token() {
  return tokenInput.value.trim();
}

function headers(extra = {}) {
  return { Authorization: `Bearer ${token()}`, ...extra };
}

async function loadContacts() {
  if (token().length < 32) {
    writeLog("token 至少 32 字符");
    return;
  }
  setBusy(true);
  try {
    const response = await fetch("/contacts", { headers: headers() });
    if (!response.ok) {
      writeLog(errorText(response.status, await response.json()));
      return;
    }
    contacts = await response.json();
    selected.clear();
    renderContacts();
    writeLog(`contacts: ${contacts.length}`);
  } finally {
    setBusy(false);
  }
}

async function sendMessage() {
  const text = textInput.value;
  const toJids = [...selected];
  if (token().length < 32) {
    writeLog("token 至少 32 字符");
    return;
  }
  if (toJids.length < 1 || toJids.length > maxRecipients) {
    writeLog(`请选择 1-${maxRecipients} 个联系人`);
    return;
  }
  if (!text.trim()) {
    writeLog("请输入文本");
    return;
  }
  setBusy(true);
  try {
    const response = await fetch("/send", {
      method: "POST",
      headers: headers({ "content-type": "application/json" }),
      body: JSON.stringify({
        to_jids: toJids,
        text,
        client_msg_id: `web-${crypto.randomUUID()}`
      })
    });
    const payload = await response.json();
    if (!response.ok) {
      writeLog(errorText(response.status, payload));
      return;
    }
    writeLog(payload.map((item) => {
      if (item.ok) {
        return `ok ${item.jid_suffix} ${item.server_msg_id || ""}`;
      }
      return `failed ${item.jid_suffix || "unknown"} ${item.error || ""}`;
    }).join("\n"));
  } finally {
    setBusy(false);
  }
}

function renderContacts() {
  contactsNode.textContent = "";
  for (const contact of contacts) {
    const jid = String(contact.jid || "");
    if (!jid) continue;
    const item = document.createElement("label");
    item.className = "contact";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = selected.has(jid);
    checkbox.disabled = !checkbox.checked && selected.size >= maxRecipients;
    checkbox.addEventListener("change", () => {
      if (checkbox.checked) {
        if (selected.size >= maxRecipients) {
          checkbox.checked = false;
          writeLog(`最多只能选择 ${maxRecipients} 个联系人`);
          return;
        }
        selected.add(jid);
      } else {
        selected.delete(jid);
      }
      renderContacts();
    });
    const text = document.createElement("span");
    text.textContent = contact.name || jid;
    const small = document.createElement("small");
    small.textContent = jid;
    text.appendChild(small);
    item.appendChild(checkbox);
    item.appendChild(text);
    contactsNode.appendChild(item);
  }
}

function errorText(status, payload) {
  if (payload && payload.error === "phone_offline") {
    return "手机离线，稍后再试";
  }
  return `error ${status}: ${(payload && payload.error) || "unknown"}`;
}

function writeLog(text) {
  logNode.textContent = text;
}

function setBusy(busy) {
  loadButton.disabled = busy;
  sendButton.disabled = busy;
}
