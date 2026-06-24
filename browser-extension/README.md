# HAI Private Chat Capture

This unpacked Chrome/Edge extension captures the currently open conversation from Robert's own ChatGPT, Gemini, Copilot, or DeepSeek account.

## Install

1. Start HAI with `docker compose --env-file .env.local -f docker-compose.local.yml up --build`.
2. Open `chrome://extensions` or `edge://extensions`.
3. Enable developer mode.
4. Choose **Load unpacked** and select this `browser-extension` folder.
5. Open a supported conversation.
6. Keep the default endpoint `http://127.0.0.1:7070/api/v1/memory-engine/import`, enter the local backend key from `.env.local`, optionally set a project, and click **Capture current conversation**.

The backend key is kept only in the open popup and is not persisted in browser storage.

## Boundaries

- Current thread only; no automatic account-wide traversal.
- Supported hosts are explicitly allowlisted in `manifest.json`.
- Requests use `credentials: omit`; account cookies are not transmitted.
- Provider DOM changes can require selector updates in `content.js`.
- Use official account exports for historical bulk backfill.
