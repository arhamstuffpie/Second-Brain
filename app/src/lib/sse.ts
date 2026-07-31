export type ServerSentEvent = {
  event: string;
  data: string;
  id?: string;
};

export class ServerSentEventParser {
  private buffer = '';

  feed(chunk: string, onEvent: (event: ServerSentEvent) => void) {
    this.buffer += chunk;
    while (true) {
      const boundary = this.buffer.match(/\r?\n\r?\n/);
      if (!boundary || boundary.index === undefined) return;
      const block = this.buffer.slice(0, boundary.index);
      this.buffer = this.buffer.slice(boundary.index + boundary[0].length);
      this.parseBlock(block, onEvent);
    }
  }

  finish(onEvent: (event: ServerSentEvent) => void) {
    const remainder = this.buffer.trim();
    this.buffer = '';
    if (remainder) this.parseBlock(remainder, onEvent);
  }

  private parseBlock(block: string, onEvent: (event: ServerSentEvent) => void) {
    let event = 'message';
    let id: string | undefined;
    const data: string[] = [];

    for (const line of block.split(/\r?\n/)) {
      if (!line || line.startsWith(':')) continue;
      const separator = line.indexOf(':');
      const field = separator === -1 ? line : line.slice(0, separator);
      let value = separator === -1 ? '' : line.slice(separator + 1);
      if (value.startsWith(' ')) value = value.slice(1);
      if (field === 'event') event = value;
      if (field === 'data') data.push(value);
      if (field === 'id') id = value;
    }

    if (data.length > 0) {
      onEvent({ event, data: data.join('\n'), id });
    }
  }
}
