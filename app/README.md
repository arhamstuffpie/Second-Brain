# Memory One mobile

Managed Expo SDK 54 application for foreground video + microphone capture,
near-realtime backend delivery, offline buffering, and Memograph access.

## Run

```bash
npm install
EXPO_PUBLIC_API_BASE_URL=http://YOUR_COMPUTER_LAN_IP:8181 npx expo start
```

Use a physical iOS or Android device for recording. `localhost` on a phone is
the phone itself, not the development computer.

## Production configuration

- Set `EXPO_PUBLIC_API_BASE_URL` to an HTTPS backend URL.
- Keep the backend CORS allow-list synchronized if a web build is served.
- Change the bundle/package identifiers in `app.json` if a different store
  identity is required.
- Build with EAS/Continuous Native Generation. This repository intentionally
  contains no generated `ios/` or `android/` prebuild projects.

## Capture behavior

- Camera video includes microphone audio.
- Recordings are split into 10, 30, or 60-second chunks.
- Each cache recording is moved to the private documents directory before it
  enters the upload queue.
- Uploads are serialized and use a stable UUID idempotency key.
- Retry state survives app restarts; uploaded files are deleted.
- Capture stops when the app leaves the foreground.
- The screen stays awake only during active capture.
- Wi-Fi-only delivery, video quality, frame sampling, and low-battery behavior
  are user-configurable.

The complete server contract is in [`../backend/API.md`](../backend/API.md).
