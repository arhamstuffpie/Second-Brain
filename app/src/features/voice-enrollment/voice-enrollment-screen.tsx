import { ScrollView, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Body, BrandMark, Button, Card, ErrorNotice, StatusPill } from '@/components/ui';
import { Fonts, MaxContentWidth, Spacing } from '@/constants/theme';
import { VoiceSampleRecorder } from '@/features/voice-enrollment/voice-sample-recorder';
import { useTheme } from '@/hooks/use-theme';
import { useApp } from '@/state/app-provider';

export function VoiceEnrollmentScreen() {
  const theme = useTheme();
  const {
    auth,
    logout,
    enrollOwnerVoice,
    refreshVoiceEnrollment,
    voiceEnrollment,
  } = useApp();

  if (voiceEnrollment.status === 'checking') {
    return (
      <SafeAreaView style={[styles.safe, { backgroundColor: theme.background }]}>
        <View style={styles.centered}>
          <BrandMark />
          <StatusPill label="checking voice setup" tone="live" />
        </View>
      </SafeAreaView>
    );
  }

  if (voiceEnrollment.status === 'error') {
    return (
      <SafeAreaView style={[styles.safe, { backgroundColor: theme.background }]}>
        <View style={styles.centered}>
          <BrandMark />
          <ErrorNotice
            title="Could not check voice setup"
            message={voiceEnrollment.error ?? 'The backend could not be reached.'}
          />
          <Button label="Try again" onPress={() => void refreshVoiceEnrollment()} />
          <Button label="Sign out" variant="ghost" onPress={() => void logout()} />
        </View>
      </SafeAreaView>
    );
  }

  return (
    <SafeAreaView style={[styles.safe, { backgroundColor: theme.background }]}>
      <ScrollView contentContainerStyle={styles.shell}>
        <View style={styles.content}>
          <BrandMark />
          <View style={styles.hero}>
            <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
              Teach Memory One your voice.
            </Text>
            <Body muted>
              This sample lets the transcription service distinguish what you said from useful
              context spoken by other people.
            </Body>
          </View>

          <Card>
            <VoiceSampleRecorder onSubmit={enrollOwnerVoice} />
          </Card>

          <Text style={[styles.privacy, { color: theme.textSecondary }]}>
            Your sample is stored privately with your account and is sent only as a speaker
            reference during diarized transcription. It is not written to Memograph as a memory.
          </Text>
          <Button
            label={`Sign out of ${auth?.user.email ?? 'this account'}`}
            variant="ghost"
            onPress={() => void logout()}
          />
        </View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1 },
  shell: {
    flexGrow: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: Spacing.lg,
    paddingVertical: Spacing.xl,
  },
  content: { width: '100%', maxWidth: Math.min(MaxContentWidth, 540), gap: Spacing.xl },
  centered: {
    flex: 1,
    alignSelf: 'center',
    width: '100%',
    maxWidth: 480,
    justifyContent: 'center',
    padding: Spacing.xl,
    gap: Spacing.xl,
  },
  hero: { gap: Spacing.sm },
  title: {
    fontFamily: Fonts.rounded,
    fontSize: 34,
    lineHeight: 38,
    letterSpacing: -1,
    fontWeight: '900',
  },
  privacy: { fontSize: 12, lineHeight: 18, textAlign: 'center' },
});
