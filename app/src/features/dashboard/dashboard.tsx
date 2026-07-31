import { useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Spacing } from '@/constants/theme';
import { ActivityScreen } from '@/features/activity/activity-screen';
import { CaptureScreen } from '@/features/capture/capture-screen';
import { ChatScreen } from '@/features/chat/chat-screen';
import { MemoryScreen } from '@/features/memory/memory-screen';
import { SettingsScreen } from '@/features/settings/settings-screen';
import { useApp } from '@/state/app-provider';
import { useTheme } from '@/hooks/use-theme';

type Tab = 'capture' | 'activity' | 'chat' | 'memory' | 'settings';

const tabs: Array<{ key: Tab; label: string }> = [
  { key: 'capture', label: 'Capture' },
  { key: 'activity', label: 'Activity' },
  { key: 'chat', label: 'Chat' },
  { key: 'memory', label: 'Memory' },
  { key: 'settings', label: 'Settings' },
];

function TabIcon({ tab, color, surface }: { tab: Tab; color: string; surface: string }) {
  if (tab === 'capture') {
    return (
      <View style={[styles.captureIcon, { borderColor: color }]}>
        <View style={[styles.captureIconCore, { backgroundColor: color }]} />
      </View>
    );
  }
  if (tab === 'activity') {
    return (
      <View style={styles.activityIcon}>
        {[8, 15, 20].map((height) => (
          <View key={height} style={[styles.activityBar, { height, backgroundColor: color }]} />
        ))}
      </View>
    );
  }
  if (tab === 'chat') {
    return (
      <View style={[styles.chatIcon, { borderColor: color }]}>
        <View style={[styles.chatTail, { borderColor: color, backgroundColor: surface }]} />
      </View>
    );
  }
  if (tab === 'memory') {
    return (
      <View style={[styles.memoryIcon, { borderColor: color }]}>
        <View style={[styles.memoryIconCore, { backgroundColor: color }]} />
      </View>
    );
  }
  return (
    <View style={styles.settingsIcon}>
      <View style={[styles.settingLine, { backgroundColor: color }]}>
        <View style={[styles.settingKnob, styles.settingKnobLeft, { backgroundColor: color }]} />
      </View>
      <View style={[styles.settingLine, { backgroundColor: color }]}>
        <View style={[styles.settingKnob, styles.settingKnobRight, { backgroundColor: color }]} />
      </View>
      <View style={[styles.settingLine, { backgroundColor: color }]}>
        <View style={[styles.settingKnob, styles.settingKnobMiddle, { backgroundColor: color }]} />
      </View>
    </View>
  );
}

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
    ) : tab === 'chat' ? (
      <ChatScreen />
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
            backgroundColor: theme.surfaceRaised,
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
                  pressed && styles.pressed,
                  disabled && styles.disabled,
                ]}>
                {active ? (
                  <View style={[styles.activeIndicator, { backgroundColor: theme.accent }]} />
                ) : null}
                <TabIcon
                  tab={item.key}
                  color={active ? theme.accent : theme.textSecondary}
                  surface={theme.surfaceRaised}
                />
                <Text
                  style={[
                    styles.navLabel,
                    { color: active ? theme.accent : theme.textSecondary },
                  ]}>
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
  navOuter: { borderTopWidth: StyleSheet.hairlineWidth, paddingTop: 3 },
  nav: {
    height: 62,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-around',
    paddingHorizontal: Spacing.sm,
  },
  navItem: {
    flex: 1,
    minHeight: 52,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 4,
  },
  activeIndicator: {
    position: 'absolute',
    top: -3,
    width: 24,
    height: 2,
    borderRadius: 1,
  },
  navLabel: { fontSize: 10, lineHeight: 12, fontWeight: '700' },
  captureIcon: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 1.8,
    alignItems: 'center',
    justifyContent: 'center',
  },
  captureIconCore: { width: 7, height: 7, borderRadius: 4 },
  activityIcon: {
    width: 21,
    height: 21,
    flexDirection: 'row',
    alignItems: 'flex-end',
    justifyContent: 'space-between',
  },
  activityBar: { width: 4, borderRadius: 2 },
  chatIcon: { width: 22, height: 17, borderRadius: 6, borderWidth: 1.8 },
  chatTail: {
    position: 'absolute',
    bottom: -3,
    left: 4,
    width: 7,
    height: 7,
    borderLeftWidth: 1.8,
    borderBottomWidth: 1.8,
    transform: [{ rotate: '-28deg' }],
  },
  memoryIcon: {
    width: 18,
    height: 18,
    borderWidth: 1.8,
    transform: [{ rotate: '45deg' }],
    alignItems: 'center',
    justifyContent: 'center',
  },
  memoryIconCore: { width: 5, height: 5, borderRadius: 3 },
  settingsIcon: { width: 22, height: 20, justifyContent: 'space-between', paddingVertical: 2 },
  settingLine: { width: 22, height: 1.5, borderRadius: 1 },
  settingKnob: { position: 'absolute', top: -2.2, width: 6, height: 6, borderRadius: 3 },
  settingKnobLeft: { left: 3 },
  settingKnobRight: { right: 3 },
  settingKnobMiddle: { left: 8 },
  pressed: { opacity: 0.58 },
  disabled: { opacity: 0.28 },
});
