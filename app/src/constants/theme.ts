import { Platform } from 'react-native';

export const Colors = {
  light: {
    text: '#191A17',
    textSecondary: '#686B62',
    background: '#F3F0E8',
    surface: '#FFFDF8',
    surfaceRaised: '#FFFFFF',
    backgroundElement: '#E9E5DC',
    backgroundSelected: '#DDD8CD',
    border: '#D8D3C8',
    accent: '#F45D2D',
    accentPressed: '#D94A1F',
    accentSoft: '#FDE0D4',
    success: '#287A50',
    successSoft: '#DCEDE3',
    warning: '#A56618',
    warningSoft: '#F6E9CF',
    danger: '#B53A32',
    dangerSoft: '#F6DEDB',
    camera: '#0D0F0D',
  },
  dark: {
    text: '#F5F2EA',
    textSecondary: '#A9ADA2',
    background: '#0D0F0D',
    surface: '#171A17',
    surfaceRaised: '#1D211D',
    backgroundElement: '#242824',
    backgroundSelected: '#30352F',
    border: '#343934',
    accent: '#FF7547',
    accentPressed: '#FF8A65',
    accentSoft: '#43271E',
    success: '#6CC895',
    successSoft: '#183427',
    warning: '#E3AD59',
    warningSoft: '#3A2B18',
    danger: '#F17D73',
    dangerSoft: '#3E211F',
    camera: '#050605',
  },
} as const;

export type AppTheme = (typeof Colors)[keyof typeof Colors];
export type ThemeColor = keyof AppTheme;

export const Fonts = Platform.select({
  ios: {
    sans: 'system-ui',
    rounded: 'ui-rounded',
    mono: 'ui-monospace',
  },
  default: {
    sans: 'sans-serif',
    rounded: 'sans-serif-medium',
    mono: 'monospace',
  },
  web: {
    sans: 'Inter, system-ui, sans-serif',
    rounded: 'Inter, system-ui, sans-serif',
    mono: 'ui-monospace, SFMono-Regular, Menlo, monospace',
  },
})!;

export const Spacing = {
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  xxl: 32,
  xxxl: 48,
} as const;

export const Radius = {
  sm: 10,
  md: 16,
  lg: 24,
  pill: 999,
} as const;

export const MaxContentWidth = 720;
