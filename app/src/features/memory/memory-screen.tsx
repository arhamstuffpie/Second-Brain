import { useState } from 'react';
import { StyleSheet, Text } from 'react-native';

import {
  Body,
  Button,
  Card,
  ChoiceRow,
  ErrorNotice,
  Field,
  PageHeader,
  Screen,
  SectionLabel,
  StatusPill,
} from '@/components/ui';
import { Fonts } from '@/constants/theme';
import { getReadableError } from '@/lib/readable-error';
import { useApp } from '@/state/app-provider';
import { useTheme } from '@/hooks/use-theme';

type MemoryMode = 'answer' | 'search' | 'graph' | 'create';

function findMemoryId(value: unknown, depth = 0): string | undefined {
  if (!value || typeof value !== 'object' || depth > 4) return undefined;
  const record = value as Record<string, unknown>;
  for (const key of ['memory_id', 'memoryId', 'id']) {
    if (typeof record[key] === 'string' && record[key]) return record[key];
  }
  for (const nested of Object.values(record)) {
    const found = findMemoryId(nested, depth + 1);
    if (found) return found;
  }
  return undefined;
}

function presentResult(value: unknown) {
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return 'Memograph returned a result that could not be displayed.';
  }
}

export function MemoryScreen() {
  const theme = useTheme();
  const { api, settings, updateSettings, showError } = useApp();
  const [mode, setMode] = useState<MemoryMode>('answer');
  const [query, setQuery] = useState('');
  const [memoryName, setMemoryName] = useState('My ambient memory');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState('');
  const [error, setError] = useState('');

  async function execute() {
    setError('');
    setResult('');
    setLoading(true);
    try {
      if (mode === 'create') {
        if (!settings.projectId.trim()) throw new Error('Add a project ID in Settings first.');
        const response = await api.memory.create(settings.projectId.trim(), {
          name: memoryName.trim(),
          memory_type: 'graph',
          graph_config: {
            mode: 'instruction',
            instruction:
              'Build a chronological personal context graph from observed activities, speech, places, objects, people, and decisions. Preserve timestamps and source modality.',
          },
        });
        const memoryId = findMemoryId(response);
        if (memoryId) await updateSettings({ memoryId });
        setResult(presentResult(response));
        return;
      }

      if (!settings.memoryId.trim()) throw new Error('Add or create a memory first.');
      if (mode !== 'graph' && !query.trim()) throw new Error('Enter a question or search query.');
      const groupId = settings.groupId.trim() || undefined;
      const response =
        mode === 'answer'
          ? await api.memory.answer(settings.memoryId, {
              query: query.trim(),
              limit: 10,
              group_id: groupId,
            })
          : mode === 'search'
            ? await api.memory.search(settings.memoryId, {
                query: query.trim(),
                limit: 10,
                group_id: groupId,
              })
            : await api.memory.graph(settings.memoryId, groupId);
      setResult(presentResult(response));
    } catch (cause) {
      const message = getReadableError(cause, 'memograph');
      setError(message);
      showError(message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <Screen contentStyle={styles.screen}>
      <PageHeader
        eyebrow="MEMOGRAPH"
        title="Memory tools"
        subtitle="Search, inspect, or create the graph shared with your captures and web app."
        action={
          <StatusPill
            label={settings.memoryId ? 'memory linked' : 'setup needed'}
            tone={settings.memoryId ? 'success' : 'warning'}
          />
        }
      />

      <ChoiceRow
        value={mode}
        onChange={setMode}
        options={[
          { label: 'Answer', value: 'answer' },
          { label: 'Search', value: 'search' },
          { label: 'Graph', value: 'graph' },
          { label: 'Create', value: 'create' },
        ]}
      />

      <Card>
        {mode === 'create' ? (
          <>
            <SectionLabel>New graph memory</SectionLabel>
            <Field
              label="Memory name"
              value={memoryName}
              onChangeText={setMemoryName}
              placeholder="My ambient memory"
            />
            <Body muted>
              Project: {settings.projectId || 'not configured'} · the returned memory ID is saved
              automatically for capture and web-app sync.
            </Body>
          </>
        ) : mode === 'graph' ? (
          <>
            <SectionLabel>Current graph</SectionLabel>
            <Body muted>
              Load the latest graph JSON for memory {settings.memoryId || 'not configured'}
              {settings.groupId ? ` in group ${settings.groupId}` : ''}.
            </Body>
          </>
        ) : (
          <>
            <SectionLabel>{mode === 'answer' ? 'Natural-language answer' : 'Semantic search'}</SectionLabel>
            <Field
              label={mode === 'answer' ? 'What do you want to remember?' : 'Search your memory'}
              value={query}
              onChangeText={setQuery}
              multiline
              placeholder={
                mode === 'answer'
                  ? 'What was I discussing before lunch?'
                  : 'meeting notes about product launch'
              }
            />
          </>
        )}
        {error ? <ErrorNotice title="Memograph request failed" message={error} /> : null}
        <Button
          label={
            mode === 'create'
              ? 'Create and link memory'
              : mode === 'graph'
                ? 'Refresh graph'
                : mode === 'answer'
                  ? 'Ask memory'
                  : 'Search'
          }
          onPress={() => void execute()}
          loading={loading}
        />
      </Card>

      {result ? (
        <Card>
          <SectionLabel>Live Memograph response</SectionLabel>
          <Text style={[styles.result, { color: theme.text }]} selectable>
            {result.slice(0, 12000)}
          </Text>
        </Card>
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  screen: { paddingBottom: 96 },
  result: { fontFamily: Fonts.mono, fontSize: 11, lineHeight: 17 },
});
