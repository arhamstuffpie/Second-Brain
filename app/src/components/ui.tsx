import { type PropsWithChildren, type ReactNode, useEffect, useRef } from 'react';
import {
  ActivityIndicator,
  Animated,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  type TextInputProps,
  type ViewStyle,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Fonts, MaxContentWidth, Radius, Spacing } from '@/constants/theme';
import { useKeyboardHeight } from '@/hooks/use-keyboard-height';
import { useTheme } from '@/hooks/use-theme';

export function BrandMark({ compact = false }: { compact?: boolean }) {
  const theme = useTheme();
  return (
    <View style={styles.brandRow}>
      <View style={[styles.brandGlyph, { backgroundColor: theme.accent }]}>
        <View style={[styles.brandGlyphCore, { backgroundColor: theme.surface }]} />
      </View>
      {!compact && <Text style={[styles.brandText, { color: theme.text }]}>MEMORY / ONE</Text>}
    </View>
  );
}

export function Screen({
  children,
  scroll = true,
  contentStyle,
}: PropsWithChildren<{ scroll?: boolean; contentStyle?: ViewStyle }>) {
  const theme = useTheme();
  const keyboardHeight = useKeyboardHeight();
  const content = (
    <View style={[styles.screenContent, contentStyle]}>
      <View style={styles.screenInner}>{children}</View>
    </View>
  );
  return (
    <SafeAreaView style={[styles.safeArea, { backgroundColor: theme.background }]} edges={['top']}>
      {scroll ? (
        <ScrollView
          style={{ backgroundColor: theme.background }}
          contentContainerStyle={[styles.scrollContent, { paddingBottom: keyboardHeight }]}
          keyboardDismissMode="interactive"
          keyboardShouldPersistTaps="handled">
          {content}
        </ScrollView>
      ) : (
        content
      )}
    </SafeAreaView>
  );
}

export function PageHeader({
  eyebrow,
  title,
  subtitle,
  action,
}: {
  eyebrow?: string;
  title: string;
  subtitle?: string;
  action?: ReactNode;
}) {
  const theme = useTheme();
  return (
    <View style={styles.header}>
      <View style={styles.headerCopy}>
        {eyebrow && <Text style={[styles.eyebrow, { color: theme.accent }]}>{eyebrow}</Text>}
        <Text style={[styles.pageTitle, { color: theme.text }]}>{title}</Text>
        {subtitle && <Text style={[styles.subtitle, { color: theme.textSecondary }]}>{subtitle}</Text>}
      </View>
      {action}
    </View>
  );
}

export function Card({
  children,
  style,
}: PropsWithChildren<{ style?: ViewStyle | ViewStyle[] }>) {
  const theme = useTheme();
  return (
    <View style={[styles.card, { backgroundColor: theme.surface, borderColor: theme.border }, style]}>
      {children}
    </View>
  );
}

export function SectionLabel({ children }: PropsWithChildren) {
  const theme = useTheme();
  return <Text style={[styles.sectionLabel, { color: theme.textSecondary }]}>{children}</Text>;
}

export function Body({
  children,
  muted = false,
  style,
}: PropsWithChildren<{ muted?: boolean; style?: object }>) {
  const theme = useTheme();
  return (
    <Text style={[styles.body, { color: muted ? theme.textSecondary : theme.text }, style]}>
      {children}
    </Text>
  );
}

export function ErrorNotice({
  message,
  title = 'Something went wrong',
}: {
  message: string;
  title?: string;
}) {
  const theme = useTheme();
  return (
    <View
      accessibilityLiveRegion="polite"
      style={[styles.errorNotice, { backgroundColor: theme.dangerSoft, borderColor: theme.danger }]}>
      <Text style={[styles.errorNoticeTitle, { color: theme.danger }]}>{title}</Text>
      <Text selectable style={[styles.errorNoticeMessage, { color: theme.danger }]}>
        {message}
      </Text>
    </View>
  );
}

export function ErrorSnackbar({
  message,
  onDismiss,
}: {
  message: string;
  onDismiss: () => void;
}) {
  const theme = useTheme();
  const opacity = useRef(new Animated.Value(0)).current;
  const offset = useRef(new Animated.Value(14)).current;

  useEffect(() => {
    Animated.parallel([
      Animated.timing(opacity, { toValue: 1, duration: 180, useNativeDriver: true }),
      Animated.timing(offset, { toValue: 0, duration: 180, useNativeDriver: true }),
    ]).start();
    const timer = setTimeout(() => {
      Animated.parallel([
        Animated.timing(opacity, { toValue: 0, duration: 180, useNativeDriver: true }),
        Animated.timing(offset, { toValue: 10, duration: 180, useNativeDriver: true }),
      ]).start(({ finished }) => {
        if (finished) onDismiss();
      });
    }, 4500);
    return () => clearTimeout(timer);
  }, [offset, onDismiss, opacity]);

  return (
    <Animated.View
      accessibilityLiveRegion="assertive"
      style={[
        styles.errorSnackbar,
        {
          backgroundColor: theme.danger,
          opacity,
          transform: [{ translateY: offset }],
        },
      ]}>
      <Text style={styles.errorSnackbarTitle}>ACTION NEEDED</Text>
      <Text selectable style={styles.errorSnackbarMessage}>
        {message}
      </Text>
      <Pressable accessibilityRole="button" onPress={onDismiss} hitSlop={10}>
        <Text style={styles.errorSnackbarDismiss}>DISMISS</Text>
      </Pressable>
    </Animated.View>
  );
}

export function Button({
  label,
  onPress,
  variant = 'primary',
  disabled,
  loading,
  compact,
}: {
  label: string;
  onPress: () => void;
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  disabled?: boolean;
  loading?: boolean;
  compact?: boolean;
}) {
  const theme = useTheme();
  const palette = {
    primary: { bg: theme.accent, fg: '#FFFFFF', border: theme.accent },
    secondary: { bg: theme.backgroundElement, fg: theme.text, border: theme.border },
    danger: { bg: theme.dangerSoft, fg: theme.danger, border: theme.dangerSoft },
    ghost: { bg: 'transparent', fg: theme.textSecondary, border: theme.border },
  }[variant];
  return (
    <Pressable
      accessibilityRole="button"
      disabled={disabled || loading}
      onPress={onPress}
      style={({ pressed }) => [
        styles.button,
        compact && styles.buttonCompact,
        { backgroundColor: palette.bg, borderColor: palette.border },
        pressed && styles.pressed,
        (disabled || loading) && styles.disabled,
      ]}>
      {loading ? (
        <ActivityIndicator color={palette.fg} size="small" />
      ) : (
        <Text style={[styles.buttonLabel, { color: palette.fg }]}>{label}</Text>
      )}
    </Pressable>
  );
}

export function Field({
  label,
  hint,
  error,
  ...props
}: TextInputProps & { label: string; hint?: string; error?: string }) {
  const theme = useTheme();
  return (
    <View style={styles.field}>
      <Text style={[styles.fieldLabel, { color: theme.text }]}>{label}</Text>
      <TextInput
        placeholderTextColor={theme.textSecondary}
        selectionColor={theme.accent}
        style={[
          styles.input,
          { color: theme.text, backgroundColor: theme.surfaceRaised, borderColor: error ? theme.danger : theme.border },
          props.multiline && styles.multiline,
        ]}
        {...props}
      />
      {(error || hint) && (
        <Text style={[styles.fieldHint, { color: error ? theme.danger : theme.textSecondary }]}>
          {error ?? hint}
        </Text>
      )}
    </View>
  );
}

export function ChoiceRow<T extends string | number>({
  options,
  value,
  onChange,
}: {
  options: Array<{ label: string; value: T }>;
  value: T;
  onChange: (value: T) => void;
}) {
  const theme = useTheme();
  return (
    <View style={styles.choiceRow}>
      {options.map((option) => {
        const selected = option.value === value;
        return (
          <Pressable
            key={String(option.value)}
            onPress={() => onChange(option.value)}
            style={({ pressed }) => [
              styles.choice,
              {
                backgroundColor: selected ? theme.accentSoft : theme.surfaceRaised,
                borderColor: selected ? theme.accent : theme.border,
              },
              pressed && styles.pressed,
            ]}>
            <Text style={[styles.choiceText, { color: selected ? theme.accent : theme.textSecondary }]}>
              {option.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

export function StatusPill({
  label,
  tone = 'neutral',
}: {
  label: string;
  tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'live';
}) {
  const theme = useTheme();
  const palette = {
    neutral: { bg: theme.backgroundElement, fg: theme.textSecondary },
    success: { bg: theme.successSoft, fg: theme.success },
    warning: { bg: theme.warningSoft, fg: theme.warning },
    danger: { bg: theme.dangerSoft, fg: theme.danger },
    live: { bg: theme.accentSoft, fg: theme.accent },
  }[tone];
  return (
    <View style={[styles.pill, { backgroundColor: palette.bg }]}>
      <View style={[styles.pillDot, { backgroundColor: palette.fg }]} />
      <Text style={[styles.pillText, { color: palette.fg }]}>{label.toUpperCase()}</Text>
    </View>
  );
}

export function Metric({ value, label }: { value: string | number; label: string }) {
  const theme = useTheme();
  return (
    <View style={styles.metric}>
      <Text style={[styles.metricValue, { color: theme.text }]}>{value}</Text>
      <Text style={[styles.metricLabel, { color: theme.textSecondary }]}>{label}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  safeArea: { flex: 1 },
  scrollContent: { flexGrow: 1 },
  screenContent: { flex: 1, alignItems: 'center', paddingHorizontal: Spacing.lg },
  screenInner: { width: '100%', maxWidth: MaxContentWidth, paddingVertical: Spacing.xl, gap: Spacing.xl },
  brandRow: { flexDirection: 'row', alignItems: 'center', gap: Spacing.md },
  brandGlyph: {
    width: 28,
    height: 28,
    borderRadius: 9,
    alignItems: 'center',
    justifyContent: 'center',
    transform: [{ rotate: '8deg' }],
  },
  brandGlyphCore: { width: 9, height: 9, borderRadius: 3 },
  brandText: { fontFamily: Fonts.mono, fontWeight: '800', fontSize: 12, letterSpacing: 1.7 },
  header: { flexDirection: 'row', gap: Spacing.lg, alignItems: 'flex-start' },
  headerCopy: { flex: 1, gap: Spacing.sm },
  eyebrow: { fontFamily: Fonts.mono, fontSize: 11, fontWeight: '800', letterSpacing: 1.5 },
  pageTitle: { fontFamily: Fonts.rounded, fontSize: 34, lineHeight: 38, fontWeight: '800', letterSpacing: -1.2 },
  subtitle: { fontSize: 15, lineHeight: 22, maxWidth: 520 },
  card: { borderWidth: StyleSheet.hairlineWidth, borderRadius: Radius.lg, padding: Spacing.lg, gap: Spacing.lg },
  sectionLabel: { fontFamily: Fonts.mono, fontSize: 11, fontWeight: '800', letterSpacing: 1.4, textTransform: 'uppercase' },
  body: { fontSize: 15, lineHeight: 22 },
  errorNotice: {
    gap: Spacing.xs,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: Radius.md,
    padding: Spacing.md,
  },
  errorNoticeTitle: { fontSize: 13, fontWeight: '900' },
  errorNoticeMessage: { fontSize: 13, lineHeight: 19 },
  errorSnackbar: {
    position: 'absolute',
    left: Spacing.lg,
    right: Spacing.lg,
    bottom: Spacing.lg,
    zIndex: 1000,
    elevation: 12,
    minHeight: 62,
    flexDirection: 'row',
    alignItems: 'center',
    gap: Spacing.md,
    borderRadius: Radius.md,
    paddingHorizontal: Spacing.lg,
    paddingVertical: Spacing.md,
    shadowColor: '#000000',
    shadowOffset: { width: 0, height: 5 },
    shadowOpacity: 0.22,
    shadowRadius: 12,
  },
  errorSnackbarTitle: { color: '#FFFFFF', fontFamily: Fonts.mono, fontSize: 9, fontWeight: '900' },
  errorSnackbarMessage: { color: '#FFFFFF', flex: 1, fontSize: 13, lineHeight: 18, fontWeight: '700' },
  errorSnackbarDismiss: { color: '#FFFFFF', fontFamily: Fonts.mono, fontSize: 9, fontWeight: '900' },
  button: {
    minHeight: 50,
    borderRadius: Radius.md,
    borderWidth: 1,
    paddingHorizontal: Spacing.xl,
    alignItems: 'center',
    justifyContent: 'center',
  },
  buttonCompact: { minHeight: 38, paddingHorizontal: Spacing.lg },
  buttonLabel: { fontSize: 15, fontWeight: '800' },
  pressed: { opacity: 0.72, transform: [{ scale: 0.99 }] },
  disabled: { opacity: 0.42 },
  field: { gap: Spacing.sm },
  fieldLabel: { fontSize: 13, fontWeight: '700' },
  input: {
    minHeight: 50,
    borderRadius: Radius.md,
    borderWidth: 1,
    paddingHorizontal: Spacing.lg,
    fontSize: 15,
  },
  multiline: { minHeight: 104, paddingTop: Spacing.md, textAlignVertical: 'top' },
  fieldHint: { fontSize: 12, lineHeight: 17 },
  choiceRow: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.sm },
  choice: { borderWidth: 1, borderRadius: Radius.pill, paddingHorizontal: Spacing.lg, paddingVertical: 10 },
  choiceText: { fontSize: 13, fontWeight: '800' },
  pill: {
    alignSelf: 'flex-start',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 7,
    borderRadius: Radius.pill,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  pillDot: { width: 6, height: 6, borderRadius: 3 },
  pillText: { fontFamily: Fonts.mono, fontSize: 9, fontWeight: '900', letterSpacing: 1 },
  metric: { gap: 2, minWidth: 72 },
  metricValue: { fontFamily: Fonts.rounded, fontSize: 24, fontWeight: '800' },
  metricLabel: { fontSize: 11, fontWeight: '700' },
});
