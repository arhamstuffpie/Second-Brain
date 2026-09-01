import { Platform } from 'react-native';

export const Colors = {
  light: {
    text: '#111315',
    textSecondary: '#62676B',
    background: '#F4F5F4',
    surface: '#FBFCFB',
    surfaceRaised: '#FFFFFF',
    backgroundElement: '#E9EBEA',
    backgroundSelected: '#DDE1E0',
    border: '#D8DCDA',
    accent: '#E9EBEA',
    accentPressed: '#E9EBEA',
    accentSoft: '#E3EDF5',
    onAccent: '#FFFFFF',
    success: '#2F6E52',
    successSoft: '#E3EFE8',
    warning: '#78643D',
    warningSoft: '#F0ECE3',
    danger: '#8B4643',
    dangerSoft: '#F3E8E7',
    camera: '#090A0B',
  },
  dark: {
    text: '#F5F7F6',
    textSecondary: '#A7ADAA',
    background: '#0B0D0E',
    surface: '#141718',
    surfaceRaised: '#1B1F20',
    backgroundElement: '#232728',
    backgroundSelected: '#2D3233',
    border: '#34393A',
    accent: '#F5F7F6',
    accentPressed: '#F5F7F6',
    accentSoft: '#1D303D',
    onAccent: '#0B1115',
    success: '#70B68F',
    successSoft: '#183126',
    warning: '#C6B17F',
    warningSoft: '#302B20',
    danger: '#D58B86',
    dangerSoft: '#382423',
    camera: '#050607',
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
  sm: 8,
  md: 12,
  lg: 18,
  pill: 999,
} as const;

export const MaxContentWidth = 720;
