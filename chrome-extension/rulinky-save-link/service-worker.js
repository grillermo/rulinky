const API_URL = "https://rulinky.chiq.me/api/links";
const AUTH_TOKEN = "44p9Wq6iJRxp4DE2vp4b3Yw6KWhRjcNohjDetwwRNy4K7cyUcxdwuWTUxVZUJkhWVjU";
const SUCCESS_BADGE_MS = 4000;

chrome.action.onClicked.addListener(async (tab) => {
  const link = tab && typeof tab.url === "string" ? tab.url : "";

  if (!link || !/^https?:\/\//i.test(link)) {
    await showFailure(tab.id, "Current tab does not have a supported URL.");
    return;
  }

  try {
    const response = await fetch(API_URL, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": AUTH_TOKEN
      },
      body: JSON.stringify({ link })
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    await chrome.action.setBadgeBackgroundColor({ color: "#15803d" });
    await chrome.action.setBadgeText({ text: "SAVED", tabId: tab.id });
    await chrome.action.setTitle({
      tabId: tab.id,
      title: "Saved to Rulinky"
    });
    await showAlert(tab.id, "Saved to Rulinky");

    setTimeout(() => {
      chrome.action.setBadgeText({ text: "", tabId: tab.id }).catch(() => {});
      chrome.action.setTitle({
        tabId: tab.id,
        title: "Save current page to Rulinky"
      }).catch(() => {});
    }, SUCCESS_BADGE_MS);
  } catch (error) {
    await showFailure(tab.id, `Rulinky save failed: ${error.message}`);
  }
});

async function showFailure(tabId, message) {
  await chrome.action.setBadgeBackgroundColor({ color: "#b91c1c" });
  await chrome.action.setBadgeText({ text: "ERR", tabId });
  await showAlert(tabId, message);
}

async function showAlert(tabId, message) {
  const [activeTab] = await chrome.tabs.query({ active: true, currentWindow: true });
  const targetTabId = tabId || (activeTab && activeTab.id);
  if (!targetTabId) return;

  await chrome.scripting.executeScript({
    target: { tabId: targetTabId },
    func: (text) => window.alert(text),
    args: [message]
  });
}
