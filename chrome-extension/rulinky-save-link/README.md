# Rulinky Chrome Extension

Local unpacked Chrome extension that saves the current tab URL to `https://rulinky.chiq.me/api/links` using the hardcoded auth token in `service-worker.js`.

## Install

1. Open `chrome://extensions`.
2. Enable **Developer mode**.
3. Click **Load unpacked**.
4. Select this folder:

```text
/Users/grillermo/c/rulinky/chrome-extension/rulinky-save-link
```

## Use

Click the extension icon on any `http` or `https` page.

- On success, the page shows an alert and the action badge briefly shows `SAVED`.
- On failure, the page shows an alert and the action badge shows `ERR`.
