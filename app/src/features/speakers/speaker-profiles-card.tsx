import { useAudioPlayer, useAudioPlayerStatus } from 'expo-audio';
import { useEffect, useState } from 'react';
import { Alert, Pressable, StyleSheet, Text, View } from 'react-native';

import { Body, Button, Card, ErrorNotice, Field, SectionLabel, StatusPill } from '@/components/ui';
import { Radius, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';
import { getReadableError } from '@/lib/readable-error';
import { useApp } from '@/state/app-provider';
import type { SpeakerProfile, SpeakerProfileInput, SpeakerRelationshipCategory } from '@/types/api';

const relationshipOptions: Array<{ label: string; value: SpeakerRelationshipCategory }> = [
  { label: 'Family', value: 'family' },
  { label: 'Friend', value: 'friend' },
  { label: 'Colleague', value: 'colleague' },
  { label: 'Professional', value: 'professional' },
  { label: 'Acquaintance', value: 'acquaintance' },
  { label: 'Other', value: 'other' },
];

function draftFor(profile: SpeakerProfile): SpeakerProfileInput {
  return {
    display_name: profile.display_name,
    relationship_category: profile.relationship_category,
    relationship_label: profile.relationship_label,
  };
}

export function SpeakerProfilesCard() {
  const theme = useTheme();
  const { api, showError } = useApp();
  const [profiles, setProfiles] = useState<SpeakerProfile[]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [error, setError] = useState('');
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState<SpeakerProfileInput | null>(null);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [playingSample, setPlayingSample] = useState<string | null>(null);
  const player = useAudioPlayer(null, { downloadFirst: true });
  const playerStatus = useAudioPlayerStatus(player);

  async function refresh() {
    setState('loading');
    setError('');
    try {
      setProfiles(await api.voice.listSpeakerProfiles());
      setState('ready');
    } catch (loadError) {
      setError(getReadableError(loadError, 'backend'));
      setState('error');
    }
  }

  useEffect(() => {
    void refresh();
  }, [api]);

  useEffect(() => {
    if (playerStatus.didJustFinish) setPlayingSample(null);
  }, [playerStatus.didJustFinish]);

  function beginEditing(profile: SpeakerProfile) {
    setEditing(profile.id);
    setDraft(draftFor(profile));
  }

  async function save(profile: SpeakerProfile) {
    if (!draft?.display_name.trim()) {
      showError('Enter a name for this speaker.');
      return;
    }
    setSaving(true);
    try {
      const updated = await api.voice.updateSpeakerProfile(profile.id, {
        display_name: draft.display_name.trim(),
        relationship_category: draft.relationship_category,
        relationship_label: draft.relationship_label.trim(),
      });
      setProfiles((current) => current.map((item) => (item.id === updated.id ? updated : item)));
      setEditing(null);
      setDraft(null);
    } catch (saveError) {
      showError(getReadableError(saveError, 'backend'));
    } finally {
      setSaving(false);
    }
  }

  async function remove(profile: SpeakerProfile) {
    setDeleting(profile.id);
    try {
      await api.voice.deleteSpeakerProfile(profile.id);
      if (editing === profile.id) {
        setEditing(null);
        setDraft(null);
      }
      setProfiles((current) => current.filter((item) => item.id !== profile.id));
    } catch (deleteError) {
      showError(getReadableError(deleteError, 'backend'));
    } finally {
      setDeleting(null);
    }
  }

  function togglePlayback(profile: SpeakerProfile) {
    const sample = profile.samples[0];
    if (!sample) return;
    if (playingSample === sample.id && playerStatus.playing) {
      player.pause();
      setPlayingSample(null);
      return;
    }
    try {
      player.replace(api.voice.speakerSampleAudioSource(profile.id, sample.id));
      player.play();
      setPlayingSample(sample.id);
    } catch (playError) {
      showError(getReadableError(playError, 'backend'));
    }
  }

  const provisionalCount = profiles.filter((profile) => profile.status === 'provisional').length;
  let profileStatusLabel = 'up to date';
  let profileStatusTone: 'live' | 'danger' | 'warning' | 'success' = 'success';
  if (state === 'loading') {
    profileStatusLabel = 'loading';
    profileStatusTone = 'live';
  } else if (state === 'error') {
    profileStatusLabel = 'unavailable';
    profileStatusTone = 'danger';
  } else if (provisionalCount > 0) {
    profileStatusLabel = `${provisionalCount} to review`;
    profileStatusTone = 'warning';
  }

  return (
    <Card>
      <View style={styles.header}>
        <View style={styles.headerCopy}>
          <SectionLabel>People in recordings</SectionLabel>
          <Body muted>
            Review new voices, play their private sample, and name them for future recordings.
          </Body>
        </View>
        <StatusPill label={profileStatusLabel} tone={profileStatusTone} />
      </View>

      {state === 'loading' ? (
        <Body muted>Loading speaker profiles…</Body>
      ) : state === 'error' ? (
        <View style={styles.stack}>
          <ErrorNotice title="Could not load speakers" message={error} />
          <Button label="Try again" variant="secondary" onPress={() => void refresh()} />
        </View>
      ) : profiles.length === 0 ? (
        <Body muted>
          New diarized voices will appear here after a recording contains at least two seconds of speech.
        </Body>
      ) : (
        <View style={styles.list}>
          {profiles.map((profile) => {
            const isEditing = editing === profile.id && draft !== null;
            const sample = profile.samples[0];
            const relationship = profile.relationship_label || profile.relationship_category;
            return (
              <View key={profile.id} style={[styles.profile, { borderTopColor: theme.border }]}>
                <View style={styles.profileHeader}>
                  <View style={styles.profileCopy}>
                    <Text style={[styles.profileName, { color: theme.text }]}>
                      {profile.display_name || 'New speaker'}
                    </Text>
                    <Body muted>
                      {relationship || (profile.status === 'provisional' ? 'Needs a name and relationship' : 'No relationship set')}
                      {' · '}{profile.sample_count} voice match{profile.sample_count === 1 ? '' : 'es'}
                    </Body>
                  </View>
                  <StatusPill
                    label={profile.status === 'provisional' ? 'new' : 'identified'}
                    tone={profile.status === 'provisional' ? 'warning' : 'success'}
                  />
                </View>

                <View style={styles.actions}>
                  <Button
                    label={playingSample === sample?.id && playerStatus.playing ? 'Pause sample' : 'Play sample'}
                    variant="secondary"
                    compact
                    disabled={!sample}
                    onPress={() => togglePlayback(profile)}
                  />
                  {!isEditing && (
                    <Button
                      label={profile.status === 'provisional' ? 'Label speaker' : 'Edit label'}
                      compact
                      onPress={() => beginEditing(profile)}
                    />
                  )}
                </View>

                {isEditing && draft && (
                  <View style={styles.editor}>
                    <Field
                      label="Name"
                      value={draft.display_name}
                      maxLength={100}
                      autoCapitalize="words"
                      placeholder="e.g. Sarah"
                      onChangeText={(display_name) => setDraft((current) => current && ({ ...current, display_name }))}
                    />
                    <View style={styles.relationships} accessibilityRole="radiogroup">
                      {relationshipOptions.map((option) => {
                        const selected = draft.relationship_category === option.value;
                        return (
                          <Pressable
                            key={option.value}
                            accessibilityRole="radio"
                            accessibilityState={{ checked: selected }}
                            onPress={() => setDraft((current) => current && ({ ...current, relationship_category: option.value }))}
                            style={({ pressed }) => [
                              styles.relationship,
                              {
                                backgroundColor: selected ? theme.accentSoft : theme.surfaceRaised,
                                borderColor: selected ? theme.accent : theme.border,
                              },
                              pressed && styles.pressed,
                            ]}>
                            <Text style={[styles.relationshipText, { color: selected ? theme.accent : theme.text }]}>
                              {option.label}
                            </Text>
                          </Pressable>
                        );
                      })}
                    </View>
                    <Field
                      label="Custom relationship (optional)"
                      value={draft.relationship_label}
                      maxLength={100}
                      placeholder="e.g. sister, manager, doctor"
                      onChangeText={(relationship_label) => setDraft((current) => current && ({ ...current, relationship_label }))}
                    />
                    <View style={styles.actions}>
                      <Button label="Save identity" compact loading={saving} onPress={() => void save(profile)} />
                      <Button
                        label="Cancel"
                        variant="secondary"
                        compact
                        disabled={saving}
                        onPress={() => {
                          setEditing(null);
                          setDraft(null);
                        }}
                      />
                      <Button
                        label={deleting === profile.id ? 'Removing…' : 'Remove voice'}
                        variant="danger"
                        compact
                        disabled={saving || deleting !== null}
                        onPress={() => Alert.alert(
                          'Remove this speaker?',
                          'Their saved sample and persistent identity will be deleted. Future speech may appear as a new speaker.',
                          [
                            { text: 'Cancel', style: 'cancel' },
                            { text: 'Remove', style: 'destructive', onPress: () => void remove(profile) },
                          ],
                        )}
                      />
                    </View>
                  </View>
                )}
              </View>
            );
          })}
        </View>
      )}
    </Card>
  );
}

const styles = StyleSheet.create({
  header: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'flex-start', gap: Spacing.lg },
  headerCopy: { flex: 1, minWidth: 220, gap: Spacing.sm },
  stack: { gap: Spacing.lg },
  list: { gap: Spacing.lg },
  profile: { borderTopWidth: StyleSheet.hairlineWidth, paddingTop: Spacing.lg, gap: Spacing.md },
  profileHeader: { flexDirection: 'row', alignItems: 'flex-start', gap: Spacing.md },
  profileCopy: { flex: 1, gap: 2 },
  profileName: { fontSize: 16, fontWeight: '800' },
  actions: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: Spacing.sm },
  editor: { gap: Spacing.lg },
  relationships: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.sm },
  relationship: {
    minHeight: 44,
    justifyContent: 'center',
    borderWidth: 1,
    borderRadius: Radius.pill,
    paddingHorizontal: Spacing.md,
  },
  relationshipText: { fontSize: 13, fontWeight: '800' },
  pressed: { opacity: 0.72 },
});
