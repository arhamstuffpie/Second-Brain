import { useEffect, useRef, useState } from 'react';
import {
  Animated,
  KeyboardAvoidingView,
  Platform,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Body, BrandMark, Button, ChoiceRow, Field } from '@/components/ui';
import { Fonts, MaxContentWidth, Radius, Spacing } from '@/constants/theme';
import { ApiError } from '@/lib/api-client';
import { useApp } from '@/state/app-provider';
import { useTheme } from '@/hooks/use-theme';

export function AuthScreen() {
  const theme = useTheme();
  const { login, signup, settings, updateSettings } = useApp();
  const [mode, setMode] = useState<'login' | 'signup'>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [apiBaseUrl, setApiBaseUrl] = useState(settings.apiBaseUrl);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const entrance = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    Animated.spring(entrance, {
      toValue: 1,
      damping: 18,
      stiffness: 130,
      useNativeDriver: true,
    }).start();
  }, [entrance]);

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
      setError(
        cause instanceof ApiError
          ? `${cause.message}${cause.requestId ? ` · ${cause.requestId}` : ''}`
          : 'Unable to authenticate. Check the backend address and connection.',
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <SafeAreaView style={[styles.safe, { backgroundColor: theme.background }]}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        style={styles.keyboard}>
        <View style={styles.shell}>
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
              <Text style={[styles.title, { color: theme.text }]}>Your day,{'\n'}remembered.</Text>
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
              {error ? <Text style={[styles.error, { color: theme.danger }]}>{error}</Text> : null}
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
        </View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1 },
  keyboard: { flex: 1 },
  shell: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: Spacing.xl },
  content: { width: '100%', maxWidth: Math.min(MaxContentWidth, 520), gap: Spacing.xl },
  hero: { gap: Spacing.md },
  title: {
    fontFamily: Fonts.rounded,
    fontSize: 47,
    lineHeight: 49,
    letterSpacing: -2,
    fontWeight: '900',
  },
  form: {
    gap: Spacing.lg,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: Radius.lg,
    padding: Spacing.xl,
  },
  error: { fontSize: 13, lineHeight: 18, fontWeight: '600' },
  privacy: { fontSize: 12, lineHeight: 18, textAlign: 'center', paddingHorizontal: Spacing.lg },
});
