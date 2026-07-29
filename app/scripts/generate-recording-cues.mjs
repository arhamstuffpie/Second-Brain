import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const sampleRate = 22_050;
const toneSeconds = 0.13;
const gapSeconds = 0.025;
const tailSeconds = 0.035;
const volume = 0.14;
const outputDirectory = join(dirname(fileURLToPath(import.meta.url)), '..', 'assets', 'sounds');

function envelope(position) {
  const attack = Math.min(1, position / 0.12);
  const release = Math.min(1, (1 - position) / 0.28);
  return Math.max(0, Math.min(attack, release));
}

function createCue(frequencies) {
  const toneSamples = Math.round(toneSeconds * sampleRate);
  const gapSamples = Math.round(gapSeconds * sampleRate);
  const tailSamples = Math.round(tailSeconds * sampleRate);
  const totalSamples = toneSamples * frequencies.length + gapSamples + tailSamples;
  const pcm = Buffer.alloc(totalSamples * 2);
  let cursor = 0;

  frequencies.forEach((frequency, index) => {
    for (let sample = 0; sample < toneSamples; sample += 1) {
      const time = sample / sampleRate;
      const position = sample / Math.max(1, toneSamples - 1);
      const fundamental = Math.sin(2 * Math.PI * frequency * time);
      const overtone = Math.sin(2 * Math.PI * frequency * 2 * time) * 0.16;
      const value = Math.round(
        Math.max(-1, Math.min(1, (fundamental + overtone) * volume * envelope(position))) * 32_767,
      );
      pcm.writeInt16LE(value, cursor * 2);
      cursor += 1;
    }
    if (index < frequencies.length - 1) cursor += gapSamples;
  });

  const header = Buffer.alloc(44);
  header.write('RIFF', 0);
  header.writeUInt32LE(36 + pcm.length, 4);
  header.write('WAVE', 8);
  header.write('fmt ', 12);
  header.writeUInt32LE(16, 16);
  header.writeUInt16LE(1, 20);
  header.writeUInt16LE(1, 22);
  header.writeUInt32LE(sampleRate, 24);
  header.writeUInt32LE(sampleRate * 2, 28);
  header.writeUInt16LE(2, 32);
  header.writeUInt16LE(16, 34);
  header.write('data', 36);
  header.writeUInt32LE(pcm.length, 40);
  return Buffer.concat([header, pcm]);
}

mkdirSync(outputDirectory, { recursive: true });
writeFileSync(join(outputDirectory, 'recording-start.wav'), createCue([560, 780]));
writeFileSync(join(outputDirectory, 'recording-stop.wav'), createCue([620, 430]));
