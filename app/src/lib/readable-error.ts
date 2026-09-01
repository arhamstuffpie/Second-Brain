import { ApiError } from '@/lib/api-client';

export type ErrorContext = 'auth' | 'backend' | 'capture' | 'memograph' | 'upload';

function cleanMessage(value: string) {
  return value
    .replace(/\s+/g, ' ')
    .replace(/service unavailable:\s*/i, '')
    .trim()
    .slice(0, 320);
}

function memographMessage(message: string) {
  const normalized = message.toLowerCase();
  if (
    normalized.includes('api key or jwt is required') ||
    normalized.includes('credentials are missing') ||
    normalized.includes('not configured')
  ) {
    return 'Memograph credentials are missing. Add an API key in Settings or configure the backend JWT.';
  }
  if (
    normalized.includes('returned 401') ||
    normalized.includes('returned 403') ||
    normalized.includes('rejected the api key') ||
    normalized.includes('invalid api key')
  ) {
    return 'Memograph rejected the API key or JWT. Check the credential and try again.';
  }
  if (normalized.includes('returned 404') || normalized.includes('could not find')) {
    return 'Memograph could not find that project or memory. Check the IDs in Settings.';
  }
  if (normalized.includes('returned 429') || normalized.includes('rate limit')) {
    return 'Memograph is receiving too many requests. Wait a moment and try again.';
  }
  if (
    normalized.includes('returned 400') ||
    normalized.includes('returned 422') ||
    normalized.includes('rejected the request')
  ) {
    return 'Memograph rejected the request. Check the project, memory, and group settings.';
  }
  if (
    normalized.includes('call memograph') ||
    normalized.includes('could not reach memograph') ||
    normalized.includes('timed out')
  ) {
    return 'The backend could not reach Memograph. Check the connection and try again.';
  }
  return 'Memograph could not complete the request. Check your credentials and memory settings, then try again.';
}

function apiMessage(error: ApiError, context: ErrorContext) {
  switch (error.code) {
    case 'NETWORK_ERROR':
      return 'Cannot reach the backend. Check the server address and make sure your phone and computer are on the same network.';
    case 'REQUEST_TIMEOUT':
      return 'The request took too long. Check your connection and try again.';
    case 'CLIENT_CONFIG':
      return 'Add a valid backend URL in Settings.';
    case 'INSECURE_BACKEND_URL':
      return 'Production connections must use a secure HTTPS backend URL.';
    case 'UNAUTHORIZED':
      return context === 'auth'
        ? 'The email or password is incorrect.'
        : 'Your session has expired. Sign in again to continue.';
    case 'FORBIDDEN':
      return 'Your account does not have permission to perform this action.';
    case 'NOT_FOUND':
      return context === 'memograph'
        ? 'The selected Memograph memory or project was not found. Check the IDs in Settings.'
        : 'The requested item could not be found. It may have been removed.';
    case 'CONFLICT':
      return 'This item changed on the server. Refresh and try again.';
    case 'UPLOAD_TOO_LARGE':
      return 'This recording is larger than the backend upload limit. Choose a lower quality or shorter chunk interval.';
    case 'VALIDATION_ERROR': {
      const detail = cleanMessage(error.message);
      return detail && !detail.toLowerCase().includes('validation')
        ? `Please check the entered information: ${detail}`
        : 'Please check the entered information and try again.';
    }
    case 'MEMOGRAPH_UNAVAILABLE':
    case 'MEMOGRAPH_ERROR':
      return memographMessage(error.message);
    case 'SERVICE_UNAVAILABLE':
      return context === 'memograph'
        ? memographMessage(error.message)
        : 'The backend is online, but a required service is unavailable. Try again shortly.';
    case 'INVALID_RESPONSE':
      return 'The backend returned an unreadable response. Update the app or check the server logs.';
    case 'INTERNAL_ERROR':
      return 'The backend encountered an unexpected problem. Try again, and check the server logs if it continues.';
    default:
      if (context === 'memograph' && error.message.toLowerCase().includes('memograph')) {
        return memographMessage(error.message);
      }
      if (error.status >= 500) {
        return 'The backend encountered a temporary problem. Try again shortly.';
      }
      return cleanMessage(error.message) || 'The request could not be completed. Please try again.';
  }
}

export function getReadableError(error: unknown, context: ErrorContext = 'backend') {
  if (error instanceof ApiError) {
    const message = apiMessage(error, context);
    return error.requestId ? `${message}\nReference: ${error.requestId}` : message;
  }
  const raw = error instanceof Error ? error.message : typeof error === 'string' ? error : '';
  const normalized = raw.toLowerCase();
  if (normalized.includes('memograph')) return memographMessage(raw);
  if (context === 'upload') {
    if (normalized.includes('transcrib') || normalized.includes('speech-to-text')) {
      return 'Audio transcription failed. Check the speech-to-text service configuration, then retry.';
    }
    if (normalized.includes('visual') || normalized.includes('vision')) {
      return 'Video analysis failed. Check the visual-analysis service configuration, then retry.';
    }
    if (normalized.includes('ffmpeg') || normalized.includes('extract')) {
      return 'The backend could not process this video file. Retry the upload or record a new chunk.';
    }
    return 'The backend could not process this chunk. Retry it, and check the server logs if it fails again.';
  }
  return cleanMessage(raw) || 'Something went wrong. Please try again.';
}
