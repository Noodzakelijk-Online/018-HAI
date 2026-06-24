const endpointInput = document.getElementById("endpoint");
const backendKeyInput = document.getElementById("backendKey");
const projectKeyInput = document.getElementById("projectKey");
const captureButton = document.getElementById("capture");
const status = document.getElementById("status");
const platform = document.getElementById("platform");

async function activeTab() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab;
}

async function initialize() {
  const saved = await chrome.storage.local.get(["endpoint", "projectKey"]);
  endpointInput.value = saved.endpoint || endpointInput.value;
  projectKeyInput.value = saved.projectKey || "";

  const tab = await activeTab();
  try {
    const page = new URL(tab.url);
    platform.textContent = page.hostname;
  } catch {
    platform.textContent = "Unsupported page";
    captureButton.disabled = true;
  }
}

captureButton.addEventListener("click", async () => {
  captureButton.disabled = true;
  status.textContent = "Reading the current conversation...";
  const config = {
    endpoint: endpointInput.value.trim(),
    backendKey: backendKeyInput.value.trim(),
    projectKey: projectKeyInput.value.trim(),
  };
  await chrome.storage.local.set({
    endpoint: config.endpoint,
    projectKey: config.projectKey,
  });

  try {
    const tab = await activeTab();
    const result = await chrome.tabs.sendMessage(tab.id, {
      type: "HAI_CAPTURE_CURRENT_CHAT",
      config,
    });
    if (!result?.ok) {
      throw new Error(result?.error || "Capture failed");
    }
    const mode = result.deduplicated ? "already current" : "stored";
    status.textContent = `${result.messageCount} messages ${mode}. ${result.insightCount} operational facts extracted.`;
  } catch (error) {
    status.textContent = error.message || String(error);
  } finally {
    captureButton.disabled = false;
  }
});

initialize();
