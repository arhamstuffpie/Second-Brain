import { BatteryState } from 'expo-battery';

export function usePowerState() {
  return {
    batteryLevel: -1,
    batteryState: BatteryState.UNKNOWN,
    lowPowerMode: false,
  };
}
