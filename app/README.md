# Memory One mobile

Managed Expo SDK 54 application for foreground video + microphone capture,
near-realtime backend delivery, offline buffering, and Memograph access.

## Account and voice setup

- A new account must enroll a clean 2–10 second owner voice sample before the
  dashboard opens. Existing accounts without a sample can continue into the
  app and receive a warning plus enrollment controls in Settings.
- Settings lists enrolled samples and lets a user add, replace, or remove their
  owner reference later.
- The sample is uploaded to the authenticated backend enrollment endpoint. It
  is used as a known-speaker reference during diarized transcription and is not
  inserted into Memograph as a memory.
- Without an enrolled sample the backend can still preserve audio, but all
  transcript speakers remain `unknown`; new-account onboarding prevents that
  degraded mode, while existing accounts receive a persistent Settings warning.
- Upload activity and retry history are stored under the authenticated user ID.
  Signing out clears the in-memory activity immediately, while that user's
  pending uploads remain available when the same account signs in again.
- Memograph IDs, credentials, capture preferences, and other Settings values
  are stored per account. Switching accounts reloads that account's Settings
  and Activity data and resets transient tab state such as open details,
  in-progress chat responses, memory results, and unsaved form drafts.

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
- Settings accepts an optional Memograph API-key override. Native builds keep it
  in SecureStore; web builds keep it only for the current browser session.
- Saving preferences shows an in-app confirmation snackbar.
- Scrollable forms add bottom space equal to the open keyboard height and
  remove it when the keyboard closes.
- Backend and Memograph failures are translated into actionable notices;
  request references remain visible for server-log troubleshooting.
- Errors also trigger a dismissible global snackbar; inline notices retain the
  details next to the action that failed.
- Expired tokens are cleared at startup, at their expiry time, or after an
  authenticated `401`, returning the user to sign-in.
- Short local chimes signal when foreground recording starts and stops.

The complete server contract is in [`../backend/API.md`](../backend/API.md).
