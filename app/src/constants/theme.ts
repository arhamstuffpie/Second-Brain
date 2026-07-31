import { Platform } from 'react-native';

export const Colors = {
  light: {
    text: '#1C1C19',
    textSecondary: '#68665F',
    background: '#F6F4EF',
    surface: '#FFFEFB',
    surfaceRaised: '#FFFFFF',
    backgroundElement: '#ECE9E2',
    backgroundSelected: '#DFDBD2',
    border: '#DDD9D0',
    accent: '#E85B32',
    accentPressed: '#C94825',
    accentSoft: '#FBE2D8',
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
    textSecondary: '#AAA99F',
    background: '#10110F',
    surface: '#181A17',
    surfaceRaised: '#20221E',
    backgroundElement: '#272923',
    backgroundSelected: '#32342D',
    border: '#383A34',
    accent: '#FF754B',
    accentPressed: '#FF8A66',
    accentSoft: '#45291F',
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
  sm: 8,
  md: 12,
  lg: 18,
  pill: 999,
} as const;

export const MaxContentWidth = 720;
