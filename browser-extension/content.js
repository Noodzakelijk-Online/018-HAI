const PLATFORM_BY_HOST = {
  "chatgpt.com": "chatgpt",
  "chat.openai.com": "chatgpt",
  "gemini.google.com": "gemini",
  "copilot.microsoft.com": "copilot",
  "chat.deepseek.com": "deepseek",
};

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message.type !== "HAI_CAPTURE_CURRENT_CHAT") {
    return false;
  }
  capture(message.config)
    .then(sendResponse)
    .catch((error) => sendResponse({ ok: false, error: error.message || String(error) }));
  return true;
});

async function capture(config) {
  const platform = PLATFORM_BY_HOST[location.hostname];
  if (!platform) {
    throw new Error("This page is not a supported AI chat platform.");
  }
  if (!config.endpoint || !config.backendKey) {
    throw new Error("HAI endpoint and backend key are required.");
  }

  const messages = extractMessages(platform);
  if (!messages.length) {
    throw new Error("No conversation messages were found. Open a thread and wait for it to finish loading.");
  }
  const payload = {
    platform,
    externalId: conversationID(),
    title: document.title.replace(/\s*[|·-]\s*(ChatGPT|Gemini|Copilot|DeepSeek).*$/i, "").trim(),
    sourceUri: location.href,
    projectKey: config.projectKey,
    capturedAt: new Date().toISOString(),
    messages,
  };
  const response = await fetch(config.endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-HAI-Backend-Key": config.backendKey,
    },
    body: JSON.stringify(payload),
    credentials: "omit",
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error || `HAI returned HTTP ${response.status}`);
  }
  return {
    ok: true,
    deduplicated: Boolean(body.deduplicated),
    messageCount: body.conversation?.messageCount || messages.length,
    insightCount: body.insights?.length || 0,
  };
}

function extractMessages(platform) {
  const platformSelectors = {
    chatgpt: [
      ['[data-message-author-role="user"]', "user"],
      ['[data-message-author-role="assistant"]', "assistant"],
    ],
    gemini: [
      ["user-query", "user"],
      ["model-response", "assistant"],
      [".query-text", "user"],
      [".response-container-content", "assistant"],
    ],
    copilot: [
      ['[data-content="user-message"]', "user"],
      ['[data-content="ai-message"]', "assistant"],
      ['[data-testid*="user-message"]', "user"],
      ['[data-testid*="assistant-message"]', "assistant"],
    ],
    deepseek: [
      ['[data-role="user"]', "user"],
      ['[data-role="assistant"]', "assistant"],
      [".ds-message--user", "user"],
      [".ds-message--assistant", "assistant"],
    ],
  };

  const discovered = [];
  const seen = new Set();
  for (const [selector, role] of platformSelectors[platform] || []) {
    for (const element of document.querySelectorAll(selector)) {
      const text = visibleText(element);
      if (!text) continue;
      const key = `${role}|${text}`;
      if (seen.has(key)) continue;
      seen.add(key);
      discovered.push({
        role,
        content: text,
        timestamp: timestampFor(element),
        position: documentPosition(element),
      });
    }
  }
  discovered.sort((left, right) => left.position - right.position);
  return discovered.map(({ position, ...message }, index) => ({
    ...message,
    externalId: `${conversationID()}-${index + 1}`,
  }));
}

function visibleText(element) {
  const clone = element.cloneNode(true);
  clone.querySelectorAll("button, nav, form, textarea, input, [aria-hidden=true]").forEach((node) => node.remove());
  return (clone.innerText || clone.textContent || "").replace(/\n{3,}/g, "\n\n").trim();
}

function timestampFor(element) {
  const time = element.querySelector("time") || element.closest("article")?.querySelector("time");
  return time?.dateTime || time?.getAttribute("datetime") || "";
}

function documentPosition(element) {
  let position = 0;
  let current = element;
  while (current) {
    let sibling = current.previousElementSibling;
    while (sibling) {
      position += 1;
      sibling = sibling.previousElementSibling;
    }
    current = current.parentElement;
    position *= 1000;
  }
  return position;
}

function conversationID() {
  const path = location.pathname.replace(/\/+$/, "");
  return path.split("/").filter(Boolean).pop() || btoa(location.href).replace(/=+$/, "");
}
