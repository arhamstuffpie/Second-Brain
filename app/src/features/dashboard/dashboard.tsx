import { useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Radius, Spacing } from '@/constants/theme';
import { ActivityScreen } from '@/features/activity/activity-screen';
import { CaptureScreen } from '@/features/capture/capture-screen';
import { MemoryScreen } from '@/features/memory/memory-screen';
import { SettingsScreen } from '@/features/settings/settings-screen';
import { useApp } from '@/state/app-provider';
import { useTheme } from '@/hooks/use-theme';

type Tab = 'capture' | 'activity' | 'memory' | 'settings';

const tabs: Array<{ key: Tab; label: string; icon: string }> = [
  { key: 'capture', label: 'Capture', icon: '●' },
  { key: 'activity', label: 'Activity', icon: '↥' },
  { key: 'memory', label: 'Memory', icon: '◇' },
  { key: 'settings', label: 'Settings', icon: '⌁' },
];

export function Dashboard() {
  const theme = useTheme();
  const insets = useSafeAreaInsets();
  const { capture } = useApp();
  const [tab, setTab] = useState<Tab>('capture');
  const captureLocked =
    capture.phase === 'starting' || capture.phase === 'capturing' || capture.phase === 'stopping';

  const content =
    tab === 'capture' ? (
      <CaptureScreen />
    ) : tab === 'activity' ? (
      <ActivityScreen />
    ) : tab === 'memory' ? (
      <MemoryScreen />
    ) : (
      <SettingsScreen />
    );

  return (
    <View style={[styles.root, { backgroundColor: theme.background }]}>
      <View style={styles.content}>{content}</View>
      <View
        style={[
          styles.navOuter,
          {
            paddingBottom: Math.max(insets.bottom, Spacing.sm),
            borderTopColor: theme.border,
            backgroundColor: theme.surface,
          },
        ]}>
        <View style={styles.nav}>
          {tabs.map((item) => {
            const active = tab === item.key;
            const disabled = captureLocked && item.key !== 'capture';
            return (
              <Pressable
                key={item.key}
                accessibilityRole="tab"
                accessibilityState={{ selected: active, disabled }}
                disabled={disabled}
                onPress={() => setTab(item.key)}
                style={({ pressed }) => [
                  styles.navItem,
                  active && { backgroundColor: theme.accentSoft },
                  pressed && styles.pressed,
                  disabled && styles.disabled,
                ]}>
                <Text style={[styles.navIcon, { color: active ? theme.accent : theme.textSecondary }]}>
                  {item.icon}
                </Text>
                <Text style={[styles.navLabel, { color: active ? theme.accent : theme.textSecondary }]}>
                  {item.label}
                </Text>
              </Pressable>
            );
          })}
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1 },
  content: { flex: 1 },
  navOuter: { borderTopWidth: StyleSheet.hairlineWidth, paddingTop: Spacing.sm },
  nav: {
    height: 58,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-around',
    paddingHorizontal: Spacing.sm,
  },
  navItem: {
    flex: 1,
    height: 50,
    borderRadius: Radius.md,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 2,
  },
  navIcon: { fontSize: 17, fontWeight: '800' },
  navLabel: { fontSize: 10, fontWeight: '800' },
  pressed: { opacity: 0.65 },
  disabled: { opacity: 0.28 },
});
