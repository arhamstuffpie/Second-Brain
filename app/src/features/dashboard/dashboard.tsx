import { useCallback, useEffect, useRef, useState } from 'react';
import { Animated, PanResponder, Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Spacing } from '@/constants/theme';
import { ActivityScreen } from '@/features/activity/activity-screen';
import { CaptureScreen } from '@/features/capture/capture-screen';
import { ChatScreen } from '@/features/chat/chat-screen';
import { MemoryScreen } from '@/features/memory/memory-screen';
import { SettingsScreen } from '@/features/settings/settings-screen';
import { useKeyboardHeight } from '@/hooks/use-keyboard-height';
import { useReducedMotion } from '@/hooks/use-reduced-motion';
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

const NAV_INSET = 5;

function withAlpha(hex: string, alpha: number) {
  const value = hex.replace('#', '');
  if (value.length !== 6) return hex;
  const red = Number.parseInt(value.slice(0, 2), 16);
  const green = Number.parseInt(value.slice(2, 4), 16);
  const blue = Number.parseInt(value.slice(4, 6), 16);
  return `rgba(${red}, ${green}, ${blue}, ${alpha})`;
}

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
  const keyboardHeight = useKeyboardHeight();
  const reducedMotion = useReducedMotion();
  const { capture } = useApp();
  const [tab, setTab] = useState<Tab>('capture');
  const [navWidth, setNavWidth] = useState(0);
  const pillX = useRef(new Animated.Value(0)).current;
  const contentTransition = useRef(new Animated.Value(1)).current;
  const dragStart = useRef(0);
  const switchTabRef = useRef<(nextTab: Tab) => void>(() => undefined);
  const snapPillRef = useRef<(index: number, velocity?: number) => void>(() => undefined);
  const captureLocked =
    capture.phase === 'starting' || capture.phase === 'capturing' || capture.phase === 'stopping';
  const hideNavigation = tab === 'chat' && keyboardHeight > 0;
  const activeIndex = tabs.findIndex((item) => item.key === tab);
  const slotWidth = navWidth > 0 ? (navWidth - NAV_INSET * 2) / tabs.length : 0;
  const navGlass = withAlpha(theme.surfaceRaised, 0.88);
  const pillGlass = withAlpha(theme.surfaceRaised, 0.82);
  const gestureMetrics = useRef({ activeIndex, captureLocked, slotWidth });
  gestureMetrics.current = { activeIndex, captureLocked, slotWidth };

  const snapPill = useCallback((index: number, velocity = 0) => {
    const width = gestureMetrics.current.slotWidth;
    if (!width) return;
    const target = index * width;
    pillX.stopAnimation();
    if (reducedMotion) {
      pillX.setValue(target);
      return;
    }
    Animated.spring(pillX, {
      toValue: target,
      velocity,
      stiffness: 300,
      damping: 28,
      mass: 0.72,
      useNativeDriver: true,
    }).start();
  }, [pillX, reducedMotion]);
  snapPillRef.current = snapPill;

  function switchTab(nextTab: Tab) {
    const nextIndex = tabs.findIndex((item) => item.key === nextTab);
    if (nextIndex < 0 || (captureLocked && nextTab !== 'capture')) {
      snapPill(activeIndex);
      return;
    }
    if (nextTab === tab) {
      snapPill(activeIndex);
      return;
    }
    if (reducedMotion) {
      contentTransition.stopAnimation();
      contentTransition.setValue(1);
      setTab(nextTab);
      return;
    }

    contentTransition.stopAnimation();
    contentTransition.setValue(0.78);
    setTab(nextTab);
    Animated.timing(contentTransition, {
      toValue: 1,
      duration: 120,
      useNativeDriver: true,
    }).start();
  }
  switchTabRef.current = switchTab;

  const panResponder = useRef(
    PanResponder.create({
      onStartShouldSetPanResponder: () => false,
      onMoveShouldSetPanResponder: (_, gesture) => {
        const metrics = gestureMetrics.current;
        return (
          !metrics.captureLocked &&
          metrics.slotWidth > 0 &&
          Math.abs(gesture.dx) > 6 &&
          Math.abs(gesture.dx) > Math.abs(gesture.dy)
        );
      },
      onMoveShouldSetPanResponderCapture: (_, gesture) => {
        const metrics = gestureMetrics.current;
        return (
          !metrics.captureLocked &&
          metrics.slotWidth > 0 &&
          Math.abs(gesture.dx) > 6 &&
          Math.abs(gesture.dx) > Math.abs(gesture.dy)
        );
      },
      onPanResponderGrant: () => {
        const metrics = gestureMetrics.current;
        dragStart.current = metrics.activeIndex * metrics.slotWidth;
        pillX.stopAnimation((value) => {
          dragStart.current = value;
        });
      },
      onPanResponderMove: (_, gesture) => {
        const { slotWidth: width } = gestureMetrics.current;
        const maximum = width * (tabs.length - 1);
        pillX.setValue(Math.max(0, Math.min(maximum, dragStart.current + gesture.dx)));
      },
      onPanResponderRelease: (_, gesture) => {
        const metrics = gestureMetrics.current;
        const projected = dragStart.current + gesture.dx + gesture.vx * metrics.slotWidth * 0.14;
        const nextIndex = Math.max(
          0,
          Math.min(tabs.length - 1, Math.round(projected / metrics.slotWidth)),
        );
        if (nextIndex === metrics.activeIndex) {
          snapPillRef.current(metrics.activeIndex, gesture.vx);
          return;
        }
        switchTabRef.current(tabs[nextIndex].key);
      },
      onPanResponderTerminate: () => {
        snapPillRef.current(gestureMetrics.current.activeIndex);
      },
      onPanResponderTerminationRequest: () => true,
      onShouldBlockNativeResponder: () => true,
    }),
  ).current;

  useEffect(() => {
    if (!slotWidth) return;
    snapPill(activeIndex);
  }, [activeIndex, snapPill, slotWidth]);

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
      <Animated.View
        style={[
          styles.content,
          {
            opacity: contentTransition,
            transform: [
              {
                translateY: contentTransition.interpolate({
                  inputRange: [0, 1],
                  outputRange: [3, 0],
                }),
              },
            ],
          },
        ]}>
        {content}
      </Animated.View>
      {!hideNavigation ? (
        <View
          style={[
            styles.navOuter,
            {
              paddingBottom: Math.max(insets.bottom, Spacing.sm),
            },
          ]}>
          <View
            style={[
              styles.navFrame,
              {
                backgroundColor: navGlass,
                borderColor: withAlpha(theme.text, 0.12),
                shadowColor: theme.text,
              },
            ]}>
            <View
              {...panResponder.panHandlers}
              onLayout={(event) => setNavWidth(event.nativeEvent.layout.width)}
              style={styles.nav}>
              {slotWidth > 0 ? (
                <Animated.View
                  pointerEvents="none"
                  style={[
                    styles.activePill,
                    {
                      width: slotWidth,
                      backgroundColor: pillGlass,
                      borderColor: withAlpha(theme.accent, 0.3),
                      shadowColor: theme.accent,
                      transform: [{ translateX: pillX }],
                    },
                  ]}>
                  <View
                    style={[styles.pillTint, { backgroundColor: withAlpha(theme.accent, 0.08) }]}
                  />
                </Animated.View>
              ) : null}
              {tabs.map((item) => {
                const active = tab === item.key;
                const disabled = captureLocked && item.key !== 'capture';
                return (
                  <Pressable
                    key={item.key}
                    accessibilityRole="tab"
                    accessibilityLabel={item.label}
                    accessibilityHint="Double tap to open, or swipe across the tab bar to change tabs."
                    accessibilityState={{ selected: active, disabled }}
                    disabled={disabled}
                    onPress={() => switchTab(item.key)}
                    style={({ pressed }) => [
                      styles.navItem,
                      pressed && styles.pressed,
                      disabled && styles.disabled,
                    ]}>
                    <TabIcon
                      tab={item.key}
                      color={active ? theme.accent : theme.textSecondary}
                      surface={active ? pillGlass : navGlass}
                    />
                    <Text
                      numberOfLines={1}
                      maxFontSizeMultiplier={1.25}
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
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1 },
  content: { flex: 1 },
  navOuter: {
    paddingTop: Spacing.sm,
    paddingHorizontal: Spacing.lg,
  },
  navFrame: {
    width: '100%',
    maxWidth: 520,
    alignSelf: 'center',
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 28,
    shadowOffset: { width: 0, height: 9 },
    shadowOpacity: 0.13,
    shadowRadius: 20,
    elevation: 10,
  },
  nav: {
    height: 66,
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: NAV_INSET,
  },
  activePill: {
    position: 'absolute',
    left: NAV_INSET,
    top: NAV_INSET,
    bottom: NAV_INSET,
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: 23,
    overflow: 'hidden',
    shadowOffset: { width: 0, height: 5 },
    shadowOpacity: 0.15,
    shadowRadius: 12,
    elevation: 5,
  },
  pillTint: {
    ...StyleSheet.absoluteFillObject,
  },
  navItem: {
    flex: 1,
    minHeight: 58,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 5,
    zIndex: 1,
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
