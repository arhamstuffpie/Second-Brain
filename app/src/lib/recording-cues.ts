import { useAudioPlayer } from 'expo-audio';
import { useCallback, useEffect } from 'react';

const START_CUE = require('../../assets/sounds/recording-start.wav');
const STOP_CUE = require('../../assets/sounds/recording-stop.wav');

export function useRecordingSoundCues() {
  const startPlayer = useAudioPlayer(START_CUE, { downloadFirst: true });
  const stopPlayer = useAudioPlayer(STOP_CUE, { downloadFirst: true });

  useEffect(() => {
    startPlayer.volume = 0.6;
    stopPlayer.volume = 0.6;
  }, [startPlayer, stopPlayer]);

  const playStarted = useCallback(() => {
    void startPlayer
      .seekTo(0)
      .then(() => startPlayer.play())
      .catch(() => undefined);
  }, [startPlayer]);

  const playStopped = useCallback(() => {
    void stopPlayer
      .seekTo(0)
      .then(() => stopPlayer.play())
      .catch(() => undefined);
  }, [stopPlayer]);

  return { playStarted, playStopped };
}
