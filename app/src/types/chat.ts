import type { MemoryAnswerStreamUsage } from '@/types/api';

export type ChatMessage = {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  createdAt: number;
  status?: 'streaming' | 'complete' | 'stopped' | 'error';
  usage?: MemoryAnswerStreamUsage;
};
