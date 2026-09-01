import { useEffect, useMemo, useState, type CSSProperties, type ChangeEvent } from 'react';

import { ApiError } from '@/lib/api-client';
import { useApp } from '@/state/app-provider';
import type { PipelineDebugProvider, PipelineDebugRun } from '@/types/api';

type Page = 'overview' | 'face' | 'speaker' | 'active-speaker';
type RunState = 'idle' | 'running' | 'success' | 'error' | 'skipped';

const pages: Array<{ id: Page; label: string; note: string }> = [
  { id: 'overview', label: 'Workflow', note: 'Safe local path' },
  { id: 'face', label: 'Face', note: 'YuNet + SFace' },
  { id: 'speaker', label: 'Speaker', note: 'ECAPA embedding' },
  { id: 'active-speaker', label: 'Active speaker', note: 'TalkNet' },
];

const defaultMetadata = JSON.stringify(
  {
    recording_id: 'debug-recording-1',
    person_tracks: [
      {
        id: 'person-track-1',
        start_time: 0,
        end_time: 5,
        tracking_confidence: 1,
        evidence_frame_ids: ['frame-1'],
        physical_presence: true,
      },
    ],
    segments: [
      {
        id: 'segment-1',
        start_time: 0,
        end_time: 5,
        speaker: 'speaker-1',
        speaker_role: 'unknown',
        text: 'Debug speech segment',
      },
    ],
  },
  null,
  2,
);

const colors = {
  page: '#09090b', panel: '#111113', raised: '#18181b', border: '#27272a',
  text: '#fafafa', muted: '#a1a1aa', purple: '#9b87f5', purpleSoft: '#211a3d',
  green: '#4ade80', greenSoft: '#10281a', red: '#fb7185', redSoft: '#32151c',
  amber: '#fbbf24', amberSoft: '#30250b', cyan: '#22d3ee',
};

export function PipelineDebugScreen() {
  const { api, auth } = useApp();
  const [page, setPage] = useState<Page>('overview');
  const [providers, setProviders] = useState<PipelineDebugProvider[]>([]);
  const [providerError, setProviderError] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [metadata, setMetadata] = useState(defaultMetadata);
  const [running, setRunning] = useState<Page | null>(null);
  const [lastStage, setLastStage] = useState<Page | null>(null);
  const [run, setRun] = useState<PipelineDebugRun | null>(null);
  const [error, setError] = useState<ApiError | Error | null>(null);
  const [raw, setRaw] = useState(false);
	const [stageStates, setStageStates] = useState<Partial<Record<Exclude<Page, 'overview'>, RunState>>>({});

  useEffect(() => {
    if (auth?.user.email !== 'admin@gmail.com') return;
    api.pipelineDebug.providers()
      .then((result) => setProviders(result.providers))
      .catch((cause) => setProviderError(cause instanceof Error ? cause.message : String(cause)));
  }, [api, auth?.user.email]);

  useEffect(() => {
    setFile(null);
    setRun(null);
    setError(null);
    setLastStage(null);
    setRaw(false);
  }, [page]);

  const providerMap = useMemo(
    () => new Map(providers.map((provider) => [provider.stage, provider])),
    [providers],
  );

  if (auth?.user.email !== 'admin@gmail.com') {
    return (
      <main style={styles.center}>
        <section style={styles.card}>
          <p style={styles.eyebrow}>Restricted</p>
          <h1 style={styles.h1}>Admin account required</h1>
          <p style={styles.muted}>Sign in as admin@gmail.com to open local pipeline diagnostics.</p>
        </section>
      </main>
    );
  }

  async function execute() {
    if (!file || page === 'overview') return;
    setRunning(page);
		setStageStates((current) => ({ ...current, [page]: 'running' }));
    setLastStage(page);
    setRun(null);
    setError(null);
    try {
      const form = new FormData();
      form.append('file', file, file.name);
      if (page === 'active-speaker') {
        JSON.parse(metadata);
        form.append('metadata', metadata);
      }
		setRun(await api.pipelineDebug.run(page, form));
		setStageStates((current) => ({ ...current, [page]: 'success' }));
    } catch (cause) {
      setError(cause instanceof Error ? cause : new Error(String(cause)));
		setStageStates((current) => ({ ...current, [page]: 'error' }));
    } finally {
      setRunning(null);
    }
  }

  const resultState: RunState = running ? 'running' : error ? 'error' : run ? 'success' : 'idle';

  return (
    <main style={styles.shell}>
      <aside style={styles.sidebar}>
        <div>
          <p style={styles.eyebrow}>Second Brain</p>
          <h1 style={styles.logo}>Pipeline Lab</h1>
          <p style={styles.small}>Local diagnostics · no graph writes</p>
        </div>
        <nav style={styles.nav} aria-label="Pipeline debug navigation">
          {pages.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => setPage(item.id)}
              style={{ ...styles.navButton, ...(page === item.id ? styles.navButtonActive : {}) }}>
              <span>{item.label}</span>
              <small style={styles.navNote}>{item.note}</small>
            </button>
          ))}
        </nav>
        <div style={styles.safeBox}>
          <span style={styles.safeDot} />
          <div>
            <strong style={styles.safeTitle}>Cost guard active</strong>
            <p style={styles.small}>STT, Vision, and Memograph are blocked.</p>
          </div>
        </div>
      </aside>

      <section style={styles.content}>
        <header style={styles.header}>
          <div>
            <p style={styles.eyebrow}>Admin diagnostics</p>
            <h2 style={styles.h1}>{pages.find((item) => item.id === page)?.label}</h2>
          </div>
          <span style={styles.adminBadge}>admin@gmail.com</span>
        </header>

        {page === 'overview' ? (
			<Overview providers={providers} error={providerError} stageStates={stageStates} />
        ) : (
          <TestPanel
            page={page}
            file={file}
            setFile={setFile}
            metadata={metadata}
            setMetadata={setMetadata}
            provider={providerMap.get(page === 'active-speaker' ? 'active_speaker' : page)}
            running={running === page}
            execute={execute}
          />
        )}

        {lastStage && page !== 'overview' ? (
          <ResultPanel state={resultState} run={run} error={error} raw={raw} setRaw={setRaw} />
        ) : null}
      </section>
    </main>
  );
}

function Overview({ providers, error, stageStates }: {
	providers: PipelineDebugProvider[];
	error: string;
	stageStates: Partial<Record<Exclude<Page, 'overview'>, RunState>>;
}) {
  const providerMap = new Map(providers.map((provider) => [provider.stage, provider]));
  const nodes: Array<{ id: string; label: string; detail: string; state: RunState }> = [
    { id: 'capture', label: 'Capture file', detail: 'Browser input', state: 'idle' },
    { id: 'split', label: 'Media split', detail: 'Isolated test input', state: 'idle' },
		{ id: 'face', label: 'Face identity', detail: providerDetail(providerMap.get('face')), state: stageStates.face ?? providerState(providerMap.get('face')) },
		{ id: 'speaker', label: 'Voice identity', detail: providerDetail(providerMap.get('speaker')), state: stageStates.speaker ?? providerState(providerMap.get('speaker')) },
		{ id: 'active_speaker', label: 'Active speaker', detail: providerDetail(providerMap.get('active_speaker')), state: stageStates['active-speaker'] ?? providerState(providerMap.get('active_speaker')) },
    { id: 'fusion', label: 'Identity fusion', detail: 'Inspect evidence only', state: 'idle' },
    { id: 'memograph', label: 'Memograph', detail: 'Hard blocked', state: 'skipped' },
  ];
  return (
    <div style={styles.stack}>
      <section style={styles.card}>
        <div style={styles.cardHeader}>
          <div>
            <p style={styles.eyebrow}>Workflow graph</p>
            <h3 style={styles.h2}>Capture identity pipeline</h3>
          </div>
          <span style={styles.zeroCost}>0 Memograph calls</span>
        </div>
        {error ? <div style={styles.errorBox}>{error}</div> : null}
        <div style={styles.workflow}>
          {nodes.map((node, index) => (
            <div key={node.id} style={styles.workflowStep}>
              <div style={{ ...styles.node, ...nodeStyle(node.state) }}>
                <span style={{ ...styles.statusDot, ...dotStyle(node.state) }} />
                <div style={styles.nodeContent}>
                  <strong>{node.label}</strong>
                  <p style={{ ...styles.small, ...styles.nodeDetail }} title={node.detail}>{node.detail}</p>
                </div>
                <span style={styles.stateLabel}>{node.state}</span>
              </div>
              {index < nodes.length - 1 ? <div style={styles.connector} /> : null}
            </div>
          ))}
        </div>
      </section>
      <section style={styles.grid3}>
        {(['face', 'speaker', 'active_speaker'] as const).map((stage) => {
          const provider = providerMap.get(stage);
          return (
            <article key={stage} style={styles.card}>
              <p style={styles.eyebrow}>{stage.replace('_', ' ')}</p>
              <h3 style={styles.modelName} title={provider?.model}>
                {provider?.model ? compactModel(provider.model) : 'Not configured'}
              </h3>
              <p style={styles.muted}>{provider?.provider || 'Waiting for backend status'}</p>
              <span style={provider?.enabled ? styles.readyBadge : styles.offBadge}>
                {provider?.enabled ? 'Configured' : 'Disabled'}
              </span>
            </article>
          );
        })}
      </section>
    </div>
  );
}

function TestPanel({
  page, file, setFile, metadata, setMetadata, provider, running, execute,
}: {
  page: Exclude<Page, 'overview'>;
  file: File | null;
  setFile: (file: File | null) => void;
  metadata: string;
  setMetadata: (value: string) => void;
  provider?: PipelineDebugProvider;
  running: boolean;
  execute: () => Promise<void>;
}) {
  const accept = page === 'face' ? 'image/*' : page === 'speaker' ? 'audio/*' : 'video/*';
  return (
    <section style={styles.card}>
      <div style={styles.cardHeader}>
        <div>
          <p style={styles.eyebrow}>Individual stage test</p>
          <h3 style={styles.h2}>{provider?.model || 'Provider disabled'}</h3>
          <p style={styles.muted}>{provider?.provider || 'Start the matching local service first.'}</p>
        </div>
        <span style={styles.localBadge}>Local · no tokens</span>
      </div>
      <label style={styles.upload}>
        <input
          type="file"
          accept={accept}
          onChange={(event: ChangeEvent<HTMLInputElement>) => setFile(event.target.files?.[0] ?? null)}
          style={styles.fileInput}
        />
        <strong>{file ? file.name : `Choose ${accept.split('/')[0]} file`}</strong>
        <span style={styles.small}>{file ? `${(file.size / 1024).toFixed(1)} KiB · ${file.type || 'unknown type'}` : 'The original bytes stay inside this debug request.'}</span>
      </label>
      {page === 'active-speaker' ? (
        <label style={styles.fieldLabel}>
          TalkNet metadata
          <textarea
            value={metadata}
            onChange={(event) => setMetadata(event.target.value)}
            spellCheck={false}
            style={styles.textarea}
          />
          <span style={styles.small}>Track and segment times must overlap the uploaded video.</span>
        </label>
      ) : null}
      <button
        type="button"
        disabled={!file || !provider?.enabled || running}
        onClick={() => void execute()}
        style={{ ...styles.runButton, ...((!file || !provider?.enabled || running) ? styles.disabled : {}) }}>
        {running ? 'Running…' : `Run ${page.replace('-', ' ')} test`}
      </button>
    </section>
  );
}

function ResultPanel({ state, run, error, raw, setRaw }: {
  state: RunState;
  run: PipelineDebugRun | null;
  error: ApiError | Error | null;
  raw: boolean;
  setRaw: (value: boolean) => void;
}) {
  return (
    <section style={styles.card}>
      <div style={styles.cardHeader}>
        <div>
          <p style={styles.eyebrow}>Execution result</p>
          <h3 style={styles.h2}>{state === 'success' ? 'Stage completed' : state === 'error' ? 'Stage failed' : 'Stage running'}</h3>
        </div>
        <span style={{ ...styles.statePill, ...nodeStyle(state) }}>{state}</span>
      </div>
      {error ? <ReadableError error={error} /> : null}
      {run ? (
        <>
          <div style={styles.metrics}>
            <Metric label="Run ID" value={run.run_id} />
            <Metric label="Duration" value={`${run.duration_ms} ms`} />
            <Metric label="Memograph" value={run.memograph_called ? 'CALLED' : 'Not called'} />
          </div>
          <div style={styles.segmented}>
            <button type="button" onClick={() => setRaw(false)} style={{ ...styles.segment, ...(!raw ? styles.segmentActive : {}) }}>Readable</button>
            <button type="button" onClick={() => setRaw(true)} style={{ ...styles.segment, ...(raw ? styles.segmentActive : {}) }}>Raw JSON</button>
          </div>
          {raw ? (
            <pre style={styles.code}>{JSON.stringify(run, null, 2)}</pre>
          ) : (
            <div style={styles.readable}>
              <JsonSection title="Data sent to the service" value={run.request} />
              <JsonSection title="Data returned by the service" value={run.response} />
            </div>
          )}
        </>
      ) : null}
    </section>
  );
}

function ReadableError({ error }: { error: ApiError | Error }) {
  const apiError = error instanceof ApiError ? error : null;
  return (
    <div style={styles.errorBox}>
      <strong>{error.message}</strong>
      <dl style={styles.errorGrid}>
        <dt>Code</dt><dd>{apiError?.code || error.name}</dd>
        <dt>HTTP</dt><dd>{apiError?.status || 'No response'}</dd>
        <dt>Request ID</dt><dd>{apiError?.requestId || 'Unavailable'}</dd>
        <dt>Retryable</dt><dd>{apiError?.retryable ? 'Yes' : 'No'}</dd>
      </dl>
    </div>
  );
}

function JsonSection({ title, value }: { title: string; value: unknown }) {
  return (
    <div style={styles.jsonSection}>
      <h4 style={styles.h3}>{title}</h4>
      <pre style={styles.code}>{JSON.stringify(value, null, 2)}</pre>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div style={styles.metric}><span style={styles.small}>{label}</span><strong>{value}</strong></div>;
}

function providerDetail(provider?: PipelineDebugProvider) {
  return provider?.enabled ? compactModel(provider.model || provider.provider) : 'Disabled';
}

function compactModel(value: string) {
  return value.split('@sha256:', 1)[0];
}

function providerState(provider?: PipelineDebugProvider): RunState {
	return provider?.enabled ? 'idle' : 'skipped';
}

function nodeStyle(state: RunState): CSSProperties {
  if (state === 'success') return { borderColor: '#245c38', background: colors.greenSoft, color: colors.green };
  if (state === 'error') return { borderColor: '#713042', background: colors.redSoft, color: colors.red };
  if (state === 'running') return { borderColor: '#725f13', background: colors.amberSoft, color: colors.amber };
  if (state === 'skipped') return { borderColor: colors.border, background: colors.page, color: colors.muted };
  return { borderColor: '#493a84', background: colors.purpleSoft, color: colors.purple };
}

function dotStyle(state: RunState): CSSProperties {
  return { background: state === 'success' ? colors.green : state === 'error' ? colors.red : state === 'running' ? colors.amber : state === 'skipped' ? colors.muted : colors.purple };
}

const styles: Record<string, CSSProperties> = {
  shell: { height: '100%', minHeight: '100vh', display: 'grid', gridTemplateColumns: '250px minmax(0, 1fr)', background: colors.page, color: colors.text, fontFamily: 'Inter, system-ui, sans-serif' },
  center: { minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24, background: colors.page, color: colors.text },
  sidebar: { borderRight: `1px solid ${colors.border}`, background: '#0d0d0f', padding: '28px 20px', display: 'flex', flexDirection: 'column', gap: 28, overflowY: 'auto' },
  content: { padding: '32px clamp(24px, 4vw, 56px) 80px', overflowY: 'auto', minWidth: 0 },
  header: { display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 16, marginBottom: 28 },
  logo: { fontSize: 20, lineHeight: 1.2, margin: '4px 0 8px' },
  h1: { fontSize: 30, lineHeight: 1.15, letterSpacing: '-0.03em', margin: '4px 0 0' },
  h2: { fontSize: 18, lineHeight: 1.3, margin: '4px 0 6px' },
  modelName: { fontSize: 18, lineHeight: 1.3, margin: '4px 0 6px', overflowWrap: 'anywhere' },
  h3: { fontSize: 14, margin: 0 },
  eyebrow: { color: colors.purple, fontSize: 11, fontWeight: 800, letterSpacing: '0.12em', textTransform: 'uppercase', margin: 0 },
  muted: { color: colors.muted, fontSize: 13, lineHeight: 1.5, margin: 0 },
  small: { color: colors.muted, fontSize: 11, lineHeight: 1.45, margin: 0 },
  nav: { display: 'grid', gap: 6 },
  navButton: { display: 'grid', gap: 2, textAlign: 'left', color: colors.muted, background: 'transparent', border: '1px solid transparent', borderRadius: 8, padding: '10px 12px', cursor: 'pointer' },
  navButtonActive: { color: colors.text, background: colors.raised, borderColor: colors.border },
  navNote: { color: colors.muted, fontSize: 10 },
  safeBox: { marginTop: 'auto', display: 'flex', gap: 10, padding: 12, background: colors.greenSoft, border: '1px solid #23452f', borderRadius: 10 },
  safeDot: { width: 8, height: 8, marginTop: 4, borderRadius: 99, background: colors.green, boxShadow: `0 0 12px ${colors.green}` },
  safeTitle: { color: colors.green, display: 'block', fontSize: 12, marginBottom: 2 },
  card: { background: colors.panel, border: `1px solid ${colors.border}`, borderRadius: 12, padding: 20, boxShadow: '0 1px 2px rgba(0,0,0,.25)' },
  cardHeader: { display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16, marginBottom: 20 },
  stack: { display: 'grid', gap: 16 },
  grid3: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(210px, 1fr))', gap: 16 },
  adminBadge: { padding: '7px 10px', borderRadius: 7, border: `1px solid ${colors.border}`, color: colors.muted, background: colors.panel, fontSize: 11 },
  zeroCost: { padding: '6px 9px', borderRadius: 99, background: colors.greenSoft, color: colors.green, border: '1px solid #245c38', fontSize: 11, fontWeight: 700 },
  localBadge: { padding: '6px 9px', borderRadius: 99, background: colors.purpleSoft, color: colors.purple, border: '1px solid #493a84', fontSize: 11, fontWeight: 700 },
  readyBadge: { display: 'inline-block', marginTop: 14, padding: '5px 8px', borderRadius: 99, background: colors.greenSoft, color: colors.green, fontSize: 11 },
  offBadge: { display: 'inline-block', marginTop: 14, padding: '5px 8px', borderRadius: 99, background: colors.raised, color: colors.muted, fontSize: 11 },
  workflow: { maxWidth: 760, display: 'grid', margin: '8px auto 0' },
  workflowStep: { display: 'grid', justifyItems: 'center' },
  node: { width: 'min(100%, 520px)', minHeight: 56, display: 'grid', gridTemplateColumns: '12px 1fr auto', alignItems: 'center', gap: 12, borderWidth: 1, borderStyle: 'solid', borderRadius: 9, padding: '10px 14px' },
  nodeContent: { minWidth: 0 },
  nodeDetail: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  statusDot: { width: 8, height: 8, borderRadius: 99, boxShadow: '0 0 10px currentColor' },
  stateLabel: { fontSize: 10, textTransform: 'uppercase', letterSpacing: '.08em' },
  connector: { width: 1, height: 20, background: colors.border },
  upload: { minHeight: 130, display: 'grid', placeItems: 'center', alignContent: 'center', gap: 5, textAlign: 'center', border: `1px dashed #49494f`, borderRadius: 10, background: colors.page, cursor: 'pointer', padding: 18 },
  fileInput: { position: 'absolute', width: 1, height: 1, opacity: 0 },
  fieldLabel: { display: 'grid', gap: 8, marginTop: 18, color: colors.text, fontSize: 12, fontWeight: 700 },
  textarea: { minHeight: 260, resize: 'vertical', background: colors.page, border: `1px solid ${colors.border}`, borderRadius: 8, color: colors.text, padding: 12, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 12, lineHeight: 1.5 },
  runButton: { width: '100%', marginTop: 18, border: 0, borderRadius: 8, background: colors.text, color: colors.page, padding: '11px 14px', fontWeight: 800, cursor: 'pointer' },
  disabled: { opacity: 0.4, cursor: 'not-allowed' },
  statePill: { borderWidth: 1, borderStyle: 'solid', borderRadius: 99, padding: '5px 9px', fontSize: 10, textTransform: 'uppercase' },
  metrics: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: 10, marginBottom: 16 },
  metric: { display: 'grid', gap: 4, background: colors.page, border: `1px solid ${colors.border}`, borderRadius: 8, padding: 12, overflow: 'hidden' },
  segmented: { display: 'inline-flex', padding: 3, borderRadius: 8, background: colors.page, border: `1px solid ${colors.border}`, marginBottom: 14 },
  segment: { border: 0, borderRadius: 6, background: 'transparent', color: colors.muted, padding: '6px 10px', cursor: 'pointer', fontSize: 11 },
  segmentActive: { background: colors.raised, color: colors.text },
  readable: { display: 'grid', gap: 12 },
  jsonSection: { minWidth: 0, display: 'grid', gap: 8 },
  code: { margin: 0, maxHeight: 480, overflow: 'auto', whiteSpace: 'pre-wrap', wordBreak: 'break-word', background: '#070708', color: '#d4d4d8', border: `1px solid ${colors.border}`, borderRadius: 8, padding: 14, fontSize: 11, lineHeight: 1.55, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' },
  errorBox: { marginBottom: 16, padding: 14, background: colors.redSoft, border: '1px solid #713042', borderRadius: 8, color: '#fecdd3', fontSize: 12 },
  errorGrid: { display: 'grid', gridTemplateColumns: '110px 1fr', gap: '5px 12px', margin: '12px 0 0', color: '#fda4af' },
};
