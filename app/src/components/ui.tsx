import type { PropsWithChildren, ReactNode } from 'react';
import {
  ActivityIndicator,
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
          contentContainerStyle={styles.scrollContent}
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
