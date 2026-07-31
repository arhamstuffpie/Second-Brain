import { useEffect, useRef, useState } from 'react';
import {
  Animated,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Body, BrandMark, Button, ChoiceRow, ErrorNotice, Field } from '@/components/ui';
import { Fonts, MaxContentWidth, Radius, Spacing } from '@/constants/theme';
import { useKeyboardHeight } from '@/hooks/use-keyboard-height';
import { useReducedMotion } from '@/hooks/use-reduced-motion';
import { getReadableError } from '@/lib/readable-error';
import { useApp } from '@/state/app-provider';
import { useTheme } from '@/hooks/use-theme';

export function AuthScreen() {
  const theme = useTheme();
  const { login, signup, settings, updateSettings, showError } = useApp();
  const [mode, setMode] = useState<'login' | 'signup'>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [apiBaseUrl, setApiBaseUrl] = useState(settings.apiBaseUrl);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const entrance = useRef(new Animated.Value(0)).current;
  const keyboardHeight = useKeyboardHeight();
  const reducedMotion = useReducedMotion();

  useEffect(() => {
    if (reducedMotion) {
      entrance.setValue(1);
      return;
    }
    Animated.spring(entrance, {
      toValue: 1,
      damping: 18,
      stiffness: 130,
      useNativeDriver: true,
    }).start();
  }, [entrance, reducedMotion]);

  async function submit() {
    setError('');
    if (!email.trim() || password.length < 8) {
      setError('Enter a valid email and a password with at least 8 characters.');
      return;
    }
    setLoading(true);
    try {
      if (apiBaseUrl.trim() !== settings.apiBaseUrl) {
        await updateSettings({ apiBaseUrl: apiBaseUrl.trim() });
      }
      if (mode === 'login') {
        await login({ email: email.trim(), password }, apiBaseUrl.trim());
      } else {
        await signup({ email: email.trim(), password }, apiBaseUrl.trim());
      }
    } catch (cause) {
      const message = getReadableError(cause, 'auth');
      setError(message);
      showError(message);
    } finally {
      setLoading(false);
    }
  }

  return (
    <SafeAreaView style={[styles.safe, { backgroundColor: theme.background }]}>
      <ScrollView
        style={styles.keyboard}
        contentContainerStyle={[styles.shell, { paddingBottom: Spacing.xl + keyboardHeight }]}
        keyboardDismissMode="interactive"
        keyboardShouldPersistTaps="handled">
          <Animated.View
            style={[
              styles.content,
              {
                opacity: entrance,
                transform: [
                  {
                    translateY: entrance.interpolate({
                      inputRange: [0, 1],
                      outputRange: [18, 0],
                    }),
                  },
                ],
              },
            ]}>
            <BrandMark />
            <View style={styles.hero}>
              <Text accessibilityRole="header" style={[styles.title, { color: theme.text }]}>
                Your day,{'\n'}remembered.
              </Text>
              <Body muted>
                Capture ambient video and audio into your private Memograph timeline.
              </Body>
            </View>

            <View style={[styles.form, { backgroundColor: theme.surface, borderColor: theme.border }]}>
              <ChoiceRow
                value={mode}
                onChange={setMode}
                options={[
                  { label: 'Sign in', value: 'login' },
                  { label: 'Create account', value: 'signup' },
                ]}
              />
              <Field
                label="Email"
                value={email}
                onChangeText={setEmail}
                keyboardType="email-address"
                autoCapitalize="none"
                autoComplete="email"
                placeholder="you@example.com"
              />
              <Field
                label="Password"
                value={password}
                onChangeText={setPassword}
                secureTextEntry
                autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                placeholder="8–72 characters"
              />
              <Field
                label="Backend URL"
                value={apiBaseUrl}
                onChangeText={setApiBaseUrl}
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
                placeholder="https://api.example.com"
                hint="Use your computer's LAN IP instead of localhost on a physical phone."
              />
              {error ? <ErrorNotice title="Unable to continue" message={error} /> : null}
              <Button
                label={mode === 'login' ? 'Continue' : 'Create private account'}
                onPress={() => void submit()}
                loading={loading}
              />
            </View>
            <Text style={[styles.privacy, { color: theme.textSecondary }]}>
              Media remains on this device until it is securely uploaded. Capture runs only while
              the app is visible.
            </Text>
          </Animated.View>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1 },
  keyboard: { flex: 1 },
  shell: { flexGrow: 1, alignItems: 'center', justifyContent: 'center', padding: 20 },
  content: { width: '100%', maxWidth: Math.min(MaxContentWidth, 500), gap: Spacing.xl },
  hero: { gap: Spacing.sm },
  title: {
    fontFamily: Fonts.rounded,
    fontSize: 40,
    lineHeight: 43,
    letterSpacing: -1.5,
    fontWeight: '900',
  },
  form: {
    gap: Spacing.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: Radius.lg,
    padding: 20,
  },
  privacy: { fontSize: 12, lineHeight: 18, textAlign: 'center', paddingHorizontal: Spacing.lg },
});
