import * as SplashScreen from 'expo-splash-screen';
import { useEffect } from 'react';
import { ActivityIndicator, StyleSheet, View } from 'react-native';

import { BrandMark } from '@/components/ui';
import { AuthScreen } from '@/features/auth/auth-screen';
import { Dashboard } from '@/features/dashboard/dashboard';
import { VoiceEnrollmentScreen } from '@/features/voice-enrollment/voice-enrollment-screen';
import { useApp } from '@/state/app-provider';
import { useTheme } from '@/hooks/use-theme';

export default function IndexScreen() {
  const theme = useTheme();
  const { ready, auth, voiceEnrollment } = useApp();

  useEffect(() => {
    if (ready) {
      void SplashScreen.hideAsync();
    }
  }, [ready]);

  if (!ready) {
    return (
      <View style={[styles.loading, { backgroundColor: theme.background }]}>
        <BrandMark />
        <ActivityIndicator color={theme.accent} />
      </View>
    );
  }

  if (!auth) return <AuthScreen />;
  if (voiceEnrollment.onboardingRequired && voiceEnrollment.status !== 'enrolled') {
    return <VoiceEnrollmentScreen />;
  }
  return <Dashboard key={auth.user.id} />;
}

const styles = StyleSheet.create({
  loading: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 20 },
});
