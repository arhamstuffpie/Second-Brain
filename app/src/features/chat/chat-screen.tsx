import { randomUUID } from 'expo-crypto';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Animated,
  FlatList,
  Keyboard,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, ErrorNotice, PageHeader, StatusPill } from '@/components/ui';
import { Fonts, MaxContentWidth, Radius, Spacing } from '@/constants/theme';
import { useReducedMotion } from '@/hooks/use-reduced-motion';
import { useTheme } from '@/hooks/use-theme';
import { ApiError } from '@/lib/api-client';
import { clearChatHistory, loadChatHistory, saveChatHistory } from '@/lib/chat-storage';
import { getReadableError } from '@/lib/readable-error';
import { useApp } from '@/state/app-provider';
import type { ChatMessage } from '@/types/chat';

const STARTERS = [
  'What have I been working on recently?',
  'Summarize the important decisions in my memory.',
  'What people, places, and tasks appear most often?',
];

const THINKING_WORDS = ['Thinking', 'through', 'your', 'memory…'];

type StreamBuffer = {
  messageId: string;
  pending: string;
  timer?: ReturnType<typeof setTimeout>;
  waiters: Array<() => void>;
};

function settleStreamWaiters(buffer: StreamBuffer) {
  const waiters = buffer.waiters.splice(0);
  waiters.forEach((resolve) => resolve());
}

function StreamingCursor() {
  const theme = useTheme();
  const reducedMotion = useReducedMotion();
  const opacity = useRef(new Animated.Value(1)).current;

  useEffect(() => {
    if (reducedMotion) {
      opacity.setValue(1);
      return;
    }
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(opacity, { toValue: 0.28, duration: 430, useNativeDriver: true }),
        Animated.timing(opacity, { toValue: 1, duration: 430, useNativeDriver: true }),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [opacity, reducedMotion]);

  return (
    <Animated.Text style={[styles.streamingCursor, { color: theme.accent, opacity }]}> ▍</Animated.Text>
  );
}

function ThinkingTextShimmer() {
  const theme = useTheme();
  const reducedMotion = useReducedMotion();
  const progress = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    progress.setValue(0);
    if (reducedMotion) return;
    const animation = Animated.loop(
      Animated.sequence([
        Animated.timing(progress, { toValue: 1, duration: 1050, useNativeDriver: true }),
        Animated.delay(180),
      ]),
    );
    animation.start();
    return () => animation.stop();
  }, [progress, reducedMotion]);

  return (
    <View
      accessible
      accessibilityLabel="Thinking through your memory"
      accessibilityLiveRegion="polite"
      style={styles.thinkingTextShimmer}>
      {THINKING_WORDS.map((word, index) => {
        const start = 0.02 + index * 0.13;
        const peak = start + 0.16;
        const end = start + 0.34;
        const opacity = reducedMotion
          ? 1
          : progress.interpolate({
              inputRange: [0, start, peak, end, 1],
              outputRange: [0.42, 0.42, 1, 0.42, 0.42],
              extrapolate: 'clamp',
            });
        return (
          <Animated.Text
            key={word}
            style={[
              styles.messageText,
              styles.thinkingWord,
              index === THINKING_WORDS.length - 1 && styles.thinkingLastWord,
              { color: theme.textSecondary, opacity },
            ]}>
            {word}
          </Animated.Text>
        );
      })}
    </View>
  );
}

function ChatTile({ message }: { message: ChatMessage }) {
  const theme = useTheme();
  const reducedMotion = useReducedMotion();
  const opacity = useRef(new Animated.Value(0)).current;
  const offset = useRef(new Animated.Value(8)).current;
  const user = message.role === 'user';
  const thinking = !user && message.status === 'streaming' && !message.content;

  useEffect(() => {
    if (reducedMotion) {
      opacity.setValue(1);
      offset.setValue(0);
      return;
    }
    Animated.parallel([
      Animated.timing(opacity, { toValue: 1, duration: 180, useNativeDriver: true }),
      Animated.timing(offset, { toValue: 0, duration: 180, useNativeDriver: true }),
    ]).start();
  }, [offset, opacity, reducedMotion]);

  return (
    <Animated.View
      style={[
        styles.messageRow,
        user ? styles.userRow : styles.assistantRow,
        { opacity, transform: [{ translateY: offset }] },
      ]}>
      <View
        style={[
          styles.messageTile,
          user ? styles.userTile : styles.assistantTile,
          {
            backgroundColor: user ? theme.accent : theme.surface,
            borderColor: user ? theme.accent : theme.border,
          },
        ]}>
        {thinking ? (
          <ThinkingTextShimmer />
        ) : (
          <Text
            selectable
            style={[
              styles.messageText,
              {
                color: user
                  ? theme.onAccent
                  : message.status === 'error'
                    ? theme.danger
                    : theme.text,
              },
            ]}>
            {message.content}
            {message.status === 'streaming' ? <StreamingCursor /> : null}
          </Text>
        )}
      </View>
      <View style={styles.messageMeta}>
        {message.status === 'streaming' ? (
          <View style={[styles.streamingDot, { backgroundColor: theme.accent }]} />
        ) : null}
        <Text style={[styles.messageTime, { color: theme.textSecondary }]}>
          {new Date(message.createdAt).toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
          })}
        </Text>
        {message.usage ? (
          <Text style={[styles.messageTime, { color: theme.textSecondary }]}>
            {message.usage.total_tokens} tokens
          </Text>
        ) : null}
        {message.status === 'stopped' ? (
          <Text style={[styles.messageTime, { color: theme.warning }]}>stopped</Text>
        ) : null}
      </View>
    </Animated.View>
  );
}

export function ChatScreen() {
  const theme = useTheme();
  const reducedMotion = useReducedMotion();
  const { api, auth, network, settings, showError } = useApp();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [memoryName, setMemoryName] = useState('');
  const [inlineError, setInlineError] = useState('');
  const [loadedHistoryIdentity, setLoadedHistoryIdentity] = useState('');
  const listRef = useRef<FlatList<ChatMessage>>(null);
  const abortRef = useRef<AbortController | null>(null);
  const messagesRef = useRef<ChatMessage[]>([]);
  const historyContextRef = useRef<{ userId: string; memoryId: string } | null>(null);
  const streamBufferRef = useRef<StreamBuffer>({
    messageId: '',
    pending: '',
    waiters: [],
  });
  const drainStreamRef = useRef<() => void>(() => undefined);
  const configuredMemoryId = settings.memoryId.trim();
  const userId = auth?.user.id ?? '';
  const historyIdentity = userId && configuredMemoryId ? `${userId}:${configuredMemoryId}` : '';
  const historyLoading = Boolean(historyIdentity && loadedHistoryIdentity !== historyIdentity);

  useEffect(
    () => () => {
      abortRef.current?.abort();
      const buffer = streamBufferRef.current;
      if (buffer.timer) clearTimeout(buffer.timer);
      if (buffer.messageId && buffer.pending) {
        messagesRef.current = messagesRef.current.map((message) =>
          message.id === buffer.messageId
            ? { ...message, content: message.content + buffer.pending }
            : message,
        );
      }
      buffer.pending = '';
      buffer.timer = undefined;
      settleStreamWaiters(buffer);
      const history = historyContextRef.current;
      if (history) {
        void saveChatHistory(history.userId, history.memoryId, messagesRef.current);
      }
    },
    [],
  );

  useEffect(() => {
    let cancelled = false;
    setLoadedHistoryIdentity('');
    setMessages([]);
    messagesRef.current = [];
    setMemoryName('');
    setInlineError('');
    historyContextRef.current = null;

    if (!historyIdentity) return () => undefined;

    void loadChatHistory(userId, configuredMemoryId)
      .then((history) => {
        if (cancelled) return;
        messagesRef.current = history;
        setMessages(history);
        historyContextRef.current = { userId, memoryId: configuredMemoryId };
        setLoadedHistoryIdentity(historyIdentity);
      })
      .catch((error) => {
        if (cancelled) return;
        const message = getReadableError(error);
        historyContextRef.current = { userId, memoryId: configuredMemoryId };
        setLoadedHistoryIdentity(historyIdentity);
        setInlineError(message);
        showError(message);
      });

    return () => {
      cancelled = true;
    };
  }, [configuredMemoryId, historyIdentity, showError, userId]);

  useEffect(() => {
    if (!historyIdentity || loadedHistoryIdentity !== historyIdentity) return;
    const timer = setTimeout(() => {
      void saveChatHistory(userId, configuredMemoryId, messages);
    }, 250);
    return () => clearTimeout(timer);
  }, [configuredMemoryId, historyIdentity, loadedHistoryIdentity, messages, userId]);

  const commitMessages = useCallback(
    (update: (current: ChatMessage[]) => ChatMessage[]) => {
      const next = update(messagesRef.current);
      messagesRef.current = next;
      setMessages(next);
    },
    [],
  );

  const updateMessage = useCallback((id: string, patch: Partial<ChatMessage>) => {
    commitMessages((current) =>
      current.map((message) => (message.id === id ? { ...message, ...patch } : message)),
    );
  }, [commitMessages]);

  const appendToken = useCallback((id: string, token: string) => {
    commitMessages((current) =>
      current.map((message) =>
        message.id === id ? { ...message, content: message.content + token } : message,
      ),
    );
  }, [commitMessages]);

  const drainStreamBuffer = useCallback(() => {
    const buffer = streamBufferRef.current;
    buffer.timer = undefined;
    if (!buffer.messageId || !buffer.pending) {
      settleStreamWaiters(buffer);
      return;
    }

    const characters = Array.from(buffer.pending);
    const revealCount = Math.max(1, Math.min(14, Math.ceil(characters.length / 10)));
    const revealed = characters.slice(0, revealCount).join('');
    buffer.pending = characters.slice(revealCount).join('');
    appendToken(buffer.messageId, revealed);

    if (buffer.pending) {
      buffer.timer = setTimeout(() => drainStreamRef.current(), 28);
    } else {
      settleStreamWaiters(buffer);
    }
  }, [appendToken]);
  drainStreamRef.current = drainStreamBuffer;

  const enqueueStreamToken = useCallback(
    (id: string, token: string) => {
      if (reducedMotion) {
        appendToken(id, token);
        return;
      }
      const buffer = streamBufferRef.current;
      if (buffer.messageId !== id) {
        if (buffer.timer) clearTimeout(buffer.timer);
        settleStreamWaiters(buffer);
        buffer.messageId = id;
        buffer.pending = '';
        buffer.timer = undefined;
      }
      buffer.pending += token;
      if (!buffer.timer) {
        buffer.timer = setTimeout(() => drainStreamRef.current(), 16);
      }
    },
    [appendToken, reducedMotion],
  );

  const flushStreamBuffer = useCallback(
    (id: string) => {
      const buffer = streamBufferRef.current;
      if (buffer.messageId !== id) return;
      if (buffer.timer) clearTimeout(buffer.timer);
      buffer.timer = undefined;
      const pending = buffer.pending;
      buffer.pending = '';
      if (pending) appendToken(id, pending);
      settleStreamWaiters(buffer);
    },
    [appendToken],
  );

  function beginStreamBuffer(id: string) {
    const buffer = streamBufferRef.current;
    if (buffer.timer) clearTimeout(buffer.timer);
    settleStreamWaiters(buffer);
    buffer.messageId = id;
    buffer.pending = '';
    buffer.timer = undefined;
  }

  function waitForStreamBuffer(id: string) {
    const buffer = streamBufferRef.current;
    if (reducedMotion || buffer.messageId !== id || (!buffer.pending && !buffer.timer)) {
      return Promise.resolve();
    }
    return new Promise<void>((resolve) => buffer.waiters.push(resolve));
  }

  async function sendMessage(value = input) {
    const query = value.trim();
    const memoryId = configuredMemoryId;
    if (!query || streaming || historyLoading) return;
    if (!memoryId) {
      const message = 'Add a Memograph memory ID in Settings and save your preferences first.';
      setInlineError(message);
      showError(message);
      return;
    }
    if (!network.online) {
      const message = 'Connect to the internet before asking your memory a question.';
      setInlineError(message);
      showError(message);
      return;
    }

    Keyboard.dismiss();
    setInput('');
    setInlineError('');
    setStreaming(true);
    const userMessage: ChatMessage = {
      id: randomUUID(),
      role: 'user',
      content: query,
      createdAt: Date.now(),
    };
    const assistantId = randomUUID();
    const assistantMessage: ChatMessage = {
      id: assistantId,
      role: 'assistant',
      content: '',
      createdAt: Date.now(),
      status: 'streaming',
    };
    commitMessages((current) => [...current, userMessage, assistantMessage]);
    beginStreamBuffer(assistantId);

    const controller = new AbortController();
    abortRef.current = controller;
    let receivedToken = false;
    try {
      await api.memory.answerStream(
        memoryId,
        {
          query,
          limit: 10,
          group_id: settings.groupId.trim() || undefined,
        },
        {
          onMeta: (meta) => setMemoryName(meta.memory_name || ''),
          onToken: (token) => {
            receivedToken = true;
            enqueueStreamToken(assistantId, token);
          },
          onUsage: (usage) => updateMessage(assistantId, { usage }),
        },
        controller.signal,
      );
      if (!receivedToken) {
        throw new ApiError(
          'Memograph completed the request without returning an answer.',
          502,
          'MEMOGRAPH_ERROR',
        );
      }
      await waitForStreamBuffer(assistantId);
      if (controller.signal.aborted) {
        throw new ApiError('The response was stopped.', 0, 'REQUEST_CANCELLED');
      }
      updateMessage(assistantId, { status: 'complete' });
    } catch (error) {
      flushStreamBuffer(assistantId);
      if (error instanceof ApiError && error.code === 'REQUEST_CANCELLED') {
        commitMessages((current) =>
          current.map((message) =>
            message.id === assistantId
              ? {
                  ...message,
                  content: message.content || 'Response stopped.',
                  status: 'stopped',
                }
              : message,
          ),
        );
      } else {
        const message = getReadableError(error, 'memograph');
        setInlineError(message);
        showError(message);
        commitMessages((current) =>
          current.map((item) =>
            item.id === assistantId
              ? {
                  ...item,
                  content: item.content || message,
                  status: 'error',
                }
              : item,
          ),
        );
      }
    } finally {
      if (streamBufferRef.current.messageId === assistantId) {
        streamBufferRef.current.messageId = '';
      }
      if (abortRef.current === controller) abortRef.current = null;
      setStreaming(false);
    }
  }

  function stopResponse() {
    abortRef.current?.abort();
    const activeMessageId = streamBufferRef.current.messageId;
    if (activeMessageId) flushStreamBuffer(activeMessageId);
  }

  function clearChat() {
    if (streaming) return;
    commitMessages(() => []);
    setInlineError('');
    setMemoryName('');
    if (userId && configuredMemoryId) {
      void clearChatHistory(userId, configuredMemoryId);
    }
  }

  return (
    <SafeAreaView style={[styles.safe, { backgroundColor: theme.background }]} edges={['top']}>
      <View style={styles.screen}>
        <View style={styles.header}>
          <PageHeader
            eyebrow="MEMOGRAPH CHAT"
            title="Ask your memory"
            subtitle={
              memoryName
                ? `Streaming answers from ${memoryName}.`
                : 'Ask questions and watch grounded answers arrive in real time.'
            }
            action={
              <StatusPill
                label={
                  streaming
                    ? 'answering'
                    : historyLoading
                      ? 'restoring'
                    : !configuredMemoryId
                      ? 'setup needed'
                      : network.online
                        ? 'ready'
                        : 'offline'
                }
                tone={
                  streaming
                    ? 'live'
                    : configuredMemoryId && network.online
                      ? 'success'
                      : 'warning'
                }
              />
            }
          />
          {messages.length > 0 ? (
            <View style={styles.clearAction}>
              <Button
                label="Clear chat"
                variant="ghost"
                compact
                disabled={streaming}
                onPress={clearChat}
              />
            </View>
          ) : null}
        </View>

        <FlatList
          ref={listRef}
          data={messages}
          keyExtractor={(message) => message.id}
          renderItem={({ item }) => <ChatTile message={item} />}
          contentContainerStyle={[
            styles.listContent,
            messages.length === 0 && styles.emptyListContent,
          ]}
          keyboardDismissMode="interactive"
          keyboardShouldPersistTaps="handled"
          onContentSizeChange={() => listRef.current?.scrollToEnd({ animated: !streaming })}
          ListEmptyComponent={
            <View style={styles.empty}>
              <View
                style={[
                  styles.emptyGlyph,
                  { backgroundColor: theme.accentSoft, borderColor: theme.accent },
                ]}>
                <Text style={[styles.emptyGlyphText, { color: theme.accent }]}>✦</Text>
              </View>
              <Text style={[styles.emptyTitle, { color: theme.text }]}>Your timeline can answer</Text>
              <Text style={[styles.emptyCopy, { color: theme.textSecondary }]}>
                {configuredMemoryId
                  ? `Try a question below. Answers are grounded in memory ${configuredMemoryId.slice(
                      0,
                      8,
                    )}…`
                  : 'Add a Memory ID in Settings and save your preferences to start chatting.'}
              </Text>
              <View style={styles.starters}>
                {STARTERS.map((starter) => (
                  <Pressable
                    key={starter}
                    accessibilityRole="button"
                    disabled={historyLoading}
                    onPress={() => void sendMessage(starter)}
                    style={({ pressed }) => [
                      styles.starter,
                      { backgroundColor: theme.surface, borderColor: theme.border },
                      pressed && styles.pressed,
                      historyLoading && styles.disabled,
                    ]}>
                    <Text style={[styles.starterText, { color: theme.text }]}>{starter}</Text>
                    <Text style={[styles.starterArrow, { color: theme.accent }]}>↗</Text>
                  </Pressable>
                ))}
              </View>
            </View>
          }
        />

        <View
          style={[
            styles.composerArea,
            {
              backgroundColor: theme.background,
              borderTopColor: theme.border,
              paddingBottom: Spacing.md,
            },
          ]}>
          {inlineError ? (
            <ErrorNotice title="Could not answer" message={inlineError} />
          ) : null}
          <View
            style={[
              styles.composer,
              { backgroundColor: theme.surface, borderColor: theme.border },
            ]}>
            <TextInput
              accessibilityLabel="Message"
              value={input}
              onChangeText={setInput}
              editable={!streaming && !historyLoading}
              multiline
              maxLength={2000}
              placeholder="Ask about your memories…"
              placeholderTextColor={theme.textSecondary}
              selectionColor={theme.accent}
              style={[styles.input, { color: theme.text }]}
            />
            {streaming ? (
              <Pressable
                accessibilityLabel="Stop response"
                accessibilityRole="button"
                onPress={stopResponse}
                style={({ pressed }) => [
                  styles.sendButton,
                  { backgroundColor: theme.backgroundElement },
                  pressed && styles.pressed,
                ]}>
                <View style={[styles.stopIcon, { backgroundColor: theme.text }]} />
              </Pressable>
            ) : (
              <Pressable
                accessibilityLabel="Send message"
                accessibilityRole="button"
                disabled={!input.trim() || !network.online || historyLoading}
                onPress={() => void sendMessage()}
                style={({ pressed }) => [
                  styles.sendButton,
                  { backgroundColor: theme.accent },
                  pressed && styles.pressed,
                  (!input.trim() || !network.online || historyLoading) && styles.disabled,
                ]}>
                <Text style={[styles.sendText, { color: theme.onAccent }]}>↑</Text>
              </Pressable>
            )}
          </View>
          <Text style={[styles.disclaimer, { color: theme.textSecondary }]}>
            Answers may be incomplete. Confirm important details in your source memories.
          </Text>
        </View>
      </View>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1 },
  screen: { flex: 1 },
  header: {
    width: '100%',
    maxWidth: MaxContentWidth,
    alignSelf: 'center',
    paddingHorizontal: Spacing.lg,
    paddingTop: 20,
    paddingBottom: Spacing.md,
    gap: Spacing.sm,
  },
  clearAction: { alignItems: 'flex-end' },
  listContent: {
    width: '100%',
    maxWidth: MaxContentWidth,
    alignSelf: 'center',
    paddingHorizontal: Spacing.lg,
    paddingVertical: Spacing.lg,
    gap: Spacing.md,
  },
  emptyListContent: { flexGrow: 1, justifyContent: 'center' },
  messageRow: { width: '100%' },
  userRow: { alignItems: 'flex-end' },
  assistantRow: { alignItems: 'flex-start' },
  messageTile: {
    maxWidth: '86%',
    borderWidth: StyleSheet.hairlineWidth,
    paddingHorizontal: 16,
    paddingVertical: 11,
  },
  userTile: {
    maxWidth: '78%',
    borderRadius: 20,
    borderBottomRightRadius: 6,
  },
  assistantTile: {
    borderRadius: 20,
    borderBottomLeftRadius: 6,
  },
  streamingDot: { width: 6, height: 6, borderRadius: 3 },
  streamingCursor: { fontSize: 14, fontWeight: '900' },
  thinkingTextShimmer: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignSelf: 'flex-start',
  },
  thinkingWord: { marginRight: Spacing.xs },
  thinkingLastWord: { marginRight: 0 },
  messageText: { fontSize: 15, lineHeight: 22 },
  messageMeta: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.sm,
    paddingHorizontal: 5,
    paddingTop: 5,
  },
  messageTime: { fontFamily: Fonts.mono, fontSize: 9, fontWeight: '600' },
  empty: { alignItems: 'center', gap: Spacing.md },
  emptyGlyph: {
    width: 48,
    height: 48,
    borderRadius: 15,
    alignItems: 'center',
    justifyContent: 'center',
  },
  emptyGlyphText: { fontSize: 22, fontWeight: '900' },
  emptyTitle: { fontFamily: Fonts.rounded, fontSize: 23, fontWeight: '800', letterSpacing: -0.4 },
  emptyCopy: { maxWidth: 420, textAlign: 'center', fontSize: 14, lineHeight: 21 },
  starters: { width: '100%', marginTop: Spacing.md, gap: Spacing.sm },
  starter: {
    minHeight: 52,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: Radius.md,
    paddingHorizontal: Spacing.lg,
    paddingVertical: Spacing.md,
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.md,
  },
  starterText: { flex: 1, fontSize: 14, lineHeight: 20, fontWeight: '700' },
  starterArrow: { fontSize: 18, fontWeight: '900' },
  composerArea: {
    width: '100%',
    borderTopWidth: StyleSheet.hairlineWidth,
    paddingHorizontal: Spacing.lg,
    paddingTop: Spacing.md,
    gap: Spacing.sm,
  },
  composer: {
    width: '100%',
    maxWidth: MaxContentWidth,
    alignSelf: 'center',
    minHeight: 56,
    maxHeight: 132,
    borderWidth: 1,
    borderRadius: Radius.lg,
    paddingLeft: Spacing.lg,
    paddingRight: Spacing.sm,
    paddingVertical: Spacing.sm,
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: Spacing.sm,
  },
  input: { flex: 1, minHeight: 40, maxHeight: 108, paddingVertical: 9, fontSize: 16, lineHeight: 22 },
  sendButton: {
    width: 42,
    height: 42,
    borderRadius: 15,
    alignItems: 'center',
    justifyContent: 'center',
  },
  sendText: { fontSize: 24, lineHeight: 27, fontWeight: '900' },
  stopIcon: { width: 13, height: 13, borderRadius: 3 },
  disclaimer: {
    width: '100%',
    maxWidth: MaxContentWidth,
    alignSelf: 'center',
    textAlign: 'center',
    fontSize: 10,
    lineHeight: 14,
  },
  pressed: { opacity: 0.7, transform: [{ scale: 0.99 }] },
  disabled: { opacity: 0.38 },
});
