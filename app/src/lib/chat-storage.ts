import AsyncStorage from '@react-native-async-storage/async-storage';

import type { ChatMessage } from '@/types/chat';

const CHAT_HISTORY_PREFIX = 'second-brain.chat-history.v1';
const MAX_SAVED_MESSAGES = 100;

function historyKey(userId: string, memoryId: string) {
  return `${CHAT_HISTORY_PREFIX}:${userId}:${memoryId}`;
}

function isChatMessage(value: unknown): value is ChatMessage {
  if (!value || typeof value !== 'object') return false;
  const message = value as Partial<ChatMessage>;
  return (
    typeof message.id === 'string' &&
    (message.role === 'user' || message.role === 'assistant') &&
    typeof message.content === 'string' &&
    typeof message.createdAt === 'number'
  );
}

function safeHistory(messages: ChatMessage[]) {
  return messages.slice(-MAX_SAVED_MESSAGES).map((message) =>
    message.status === 'streaming'
      ? {
          ...message,
          content: message.content || 'Response stopped.',
          status: 'stopped' as const,
        }
      : message,
  );
}

export async function loadChatHistory(userId: string, memoryId: string) {
  const stored = await AsyncStorage.getItem(historyKey(userId, memoryId));
  if (!stored) return [];

  try {
    const parsed = JSON.parse(stored) as unknown;
    return Array.isArray(parsed) ? safeHistory(parsed.filter(isChatMessage)) : [];
  } catch {
    return [];
  }
}

export async function saveChatHistory(
  userId: string,
  memoryId: string,
  messages: ChatMessage[],
) {
  const key = historyKey(userId, memoryId);
  if (messages.length === 0) {
    await AsyncStorage.removeItem(key);
    return;
  }
  await AsyncStorage.setItem(key, JSON.stringify(safeHistory(messages)));
}

export async function clearChatHistory(userId: string, memoryId: string) {
  await AsyncStorage.removeItem(historyKey(userId, memoryId));
}
