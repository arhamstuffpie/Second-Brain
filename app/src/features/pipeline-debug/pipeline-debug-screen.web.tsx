import { useEffect, useMemo, useState, type CSSProperties, type ChangeEvent } from 'react';

import { ApiError } from '@/lib/api-client';
import type { ApiClient } from '@/lib/api-client';
import { useApp } from '@/state/app-provider';
import type {
  PipelineDebugAnalysisOverview,
  PipelineDebugDenseObservation,
  PipelineDebugDenseOverview,
  PipelineDebugDenseRecording,
  PipelineDebugDenseRecordingDetail,
  PipelineDebugDenseTrack,
  PipelineDebugFusionEvidence,
  PipelineDebugOwner,
  PipelineDebugProvider,
  PipelineDebugRun,
} from '@/types/api';

type TestPage = 'face' | 'speaker' | 'active-speaker';
type Page = 'overview' | 'dense' | TestPage;
type RunState = 'idle' | 'running' | 'success' | 'error' | 'skipped';

const pages: Array<{ id: Page; label: string; note: string }> = [
  { id: 'overview', label: 'Workflow', note: 'Safe local path' },
  { id: 'dense', label: 'Dense tracking', note: 'Worker + evidence' },
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
  const [owners, setOwners] = useState<PipelineDebugOwner[]>([]);
  const [selectedOwnerID, setSelectedOwnerID] = useState('');
  const [ownerError, setOwnerError] = useState('');
  const [providerError, setProviderError] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [metadata, setMetadata] = useState(defaultMetadata);
  const [running, setRunning] = useState<TestPage | null>(null);
  const [lastStage, setLastStage] = useState<TestPage | null>(null);
  const [run, setRun] = useState<PipelineDebugRun | null>(null);
  const [error, setError] = useState<ApiError | Error | null>(null);
  const [raw, setRaw] = useState(false);
	const [stageStates, setStageStates] = useState<Partial<Record<TestPage, RunState>>>({});

  useEffect(() => {
    if (auth?.user.email !== 'admin@gmail.com') return;
    api.pipelineDebug.providers()
      .then((result) => setProviders(result.providers))
      .catch((cause) => setProviderError(cause instanceof Error ? cause.message : String(cause)));
    api.pipelineDebug.owners()
      .then((result) => {
        setOwners(result.owners);
        setSelectedOwnerID((current) => current || (
          result.owners.find((owner) => owner.run_count > 0)
          ?? result.owners.find((owner) => owner.email === 'admin@gmail.com')
          ?? result.owners[0]
        )?.id || '');
      })
      .catch((cause) => setOwnerError(cause instanceof Error ? cause.message : String(cause)));
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
  const selectedOwner = owners.find((owner) => owner.id === selectedOwnerID);

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
    if (!file || page === 'overview' || page === 'dense') return;
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
    <main className="pipeline-debug-shell" style={styles.shell}>
      <style>{responsiveStyles}</style>
      <aside className="pipeline-debug-sidebar" style={styles.sidebar}>
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

      <section className="pipeline-debug-content" style={styles.content}>
        <header className="pipeline-debug-header" style={styles.header}>
          <div>
            <p style={styles.eyebrow}>Admin diagnostics</p>
            <h2 style={styles.h1}>{pages.find((item) => item.id === page)?.label}</h2>
          </div>
          <div className="pipeline-debug-owner-controls" style={styles.ownerControls}>
            <label style={styles.ownerLabel}>
              Evidence owner
              <select
                aria-label="Evidence owner"
                value={selectedOwnerID}
                onChange={(event) => setSelectedOwnerID(event.target.value)}
                disabled={owners.length === 0}
                style={styles.ownerSelect}>
                {owners.length === 0 ? <option value="">No owners available</option> : null}
                {owners.map((owner) => (
                  <option key={owner.id} value={owner.id}>
                    {owner.email} — {owner.recording_count} recordings
                  </option>
                ))}
              </select>
            </label>
            <span style={styles.adminBadge}>Signed in: admin@gmail.com</span>
          </div>
        </header>
        {ownerError ? <div style={styles.errorBox}>{ownerError}</div> : null}

        {page === 'overview' ? (
			<Overview
            api={api}
            ownerUserID={selectedOwnerID}
            ownerEmail={selectedOwner?.email || 'No owner selected'}
            providers={providers}
            error={providerError}
            stageStates={stageStates}
          />
        ) : page === 'dense' ? (
          <DensePanel
            api={api}
            ownerUserID={selectedOwnerID}
            ownerEmail={selectedOwner?.email || 'No owner selected'}
            provider={providerMap.get('dense_person_tracking')}
          />
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

        {lastStage && page === lastStage ? (
          <ResultPanel state={resultState} run={run} error={error} raw={raw} setRaw={setRaw} />
        ) : null}
      </section>
    </main>
  );
}

function Overview({ api, ownerUserID, ownerEmail, providers, error, stageStates }: {
	api: ApiClient;
	ownerUserID: string;
	ownerEmail: string;
	providers: PipelineDebugProvider[];
	error: string;
	stageStates: Partial<Record<TestPage, RunState>>;
}) {
  const [analysis, setAnalysis] = useState<PipelineDebugAnalysisOverview | null>(null);
  const [analysisError, setAnalysisError] = useState('');
  const [analysisLoading, setAnalysisLoading] = useState(false);
  useEffect(() => {
    if (!ownerUserID) {
      setAnalysis(null);
      return;
    }
    let active = true;
    setAnalysis(null);
    setAnalysisError('');
    setAnalysisLoading(true);
    api.pipelineDebug.analysisOverview(ownerUserID)
      .then((result) => active && setAnalysis(result))
      .catch((cause) => active && setAnalysisError(cause instanceof Error ? cause.message : String(cause)))
      .finally(() => active && setAnalysisLoading(false));
    return () => { active = false; };
  }, [api, ownerUserID]);
  const providerMap = new Map(providers.map((provider) => [provider.stage, provider]));
  const nodes: Array<{ id: string; label: string; detail: string; state: RunState }> = [
    { id: 'capture', label: 'Capture file', detail: 'Browser input', state: 'idle' },
    { id: 'split', label: 'Media split', detail: 'Isolated test input', state: 'idle' },
		{ id: 'face', label: 'Face identity', detail: providerDetail(providerMap.get('face')), state: stageStates.face ?? providerState(providerMap.get('face')) },
		{ id: 'dense', label: 'Dense face tracking', detail: providerDetail(providerMap.get('dense_person_tracking')), state: providerState(providerMap.get('dense_person_tracking')) },
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
        {(['face', 'dense_person_tracking', 'speaker', 'active_speaker'] as const).map((stage) => {
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
      <section style={styles.card}>
        <div style={styles.cardHeader}>
          <div>
            <p style={styles.eyebrow}>Persisted pipeline</p>
            <h3 style={styles.h2}>{ownerEmail}</h3>
            <p style={styles.muted}>Recent analysis versions and every durable stage stored for this owner.</p>
          </div>
          <span style={styles.adminBadge}>{analysis?.runs.length ?? 0} runs</span>
        </div>
        {analysisLoading ? <p style={styles.muted} aria-live="polite">Loading pipeline runs…</p> : null}
        {analysisError ? <div style={styles.errorBox}>{analysisError}</div> : null}
        {analysis && analysis.runs.length === 0 ? <p style={styles.muted}>No persisted analysis runs for this owner.</p> : null}
        {analysis?.runs.length ? (
          <div style={styles.runList}>
            {analysis.runs.map((item) => (
              <article key={item.id} style={styles.runItem}>
                <div style={styles.cardHeader}>
                  <div>
                    <strong>{item.file_name}</strong>
                    <p style={styles.small}>{item.recording_id} · version {item.processing_version} · {item.configuration_profile}</p>
                  </div>
                  <span style={{ ...styles.statePill, ...nodeStyle(stageRunState(item.status)) }}>
                    {item.status}{item.active ? ' · active' : ''}
                  </span>
                </div>
                {item.last_error ? <p style={styles.recordingError}>{item.last_error}</p> : null}
                <div style={styles.stageGrid}>
                  {item.stages.map((stage) => (
                    <details key={stage.stage} style={styles.stageItem}>
                      <summary style={styles.stageSummary}>
                        <span>{stage.stage.replaceAll('_', ' ')}</span>
                        <span style={{ color: nodeStyle(stageRunState(stage.status)).color }}>
                          {stage.status} · {stage.attempts}/{stage.max_attempts}
                        </span>
                      </summary>
                      <dl style={styles.definitionGrid}>
                        <dt>Required</dt><dd>{stage.required ? 'Yes' : 'No'}</dd>
                        <dt>Depends on</dt><dd>{stage.depends_on.join(', ') || 'None'}</dd>
                        <dt>Scheduled</dt><dd>{formatDate(stage.run_at)}</dd>
                        <dt>Updated</dt><dd>{formatDate(stage.updated_at)}</dd>
                      </dl>
                      {stage.last_error ? <p style={styles.recordingError}>{stage.last_error}</p> : null}
                      <pre style={styles.code}>{JSON.stringify({
                        checkpoint: stage.checkpoint,
                        result_provenance: stage.result_provenance,
                      }, null, 2)}</pre>
                    </details>
                  ))}
                </div>
              </article>
            ))}
          </div>
        ) : null}
      </section>
    </div>
  );
}

function DensePanel({ api, ownerUserID, ownerEmail, provider }: {
  api: ApiClient;
  ownerUserID: string;
  ownerEmail: string;
  provider?: PipelineDebugProvider;
}) {
  const [overview, setOverview] = useState<PipelineDebugDenseOverview | null>(null);
  const [selectedRecording, setSelectedRecording] = useState<PipelineDebugDenseRecording | null>(null);
  const [detail, setDetail] = useState<PipelineDebugDenseRecordingDetail | null>(null);
  const [selectedTrackID, setSelectedTrackID] = useState('');
  const [selectedObservationID, setSelectedObservationID] = useState('');
  const [refresh, setRefresh] = useState(0);
  const [overviewLoading, setOverviewLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!ownerUserID) {
      setOverview(null);
      setSelectedRecording(null);
      setOverviewLoading(false);
      return;
    }
    let active = true;
    setOverviewLoading(true);
    setError(null);
    setOverview(null);
    setSelectedRecording(null);
    setDetail(null);
    api.pipelineDebug.denseOverview(ownerUserID)
      .then((result) => {
        if (!active) return;
        setOverview(result);
        setSelectedRecording((current) => {
          if (!current) return result.recordings[0] ?? null;
          return result.recordings.find((recording) => (
            recording.recording_id === current.recording_id
            && recording.processing_version === current.processing_version
          )) ?? result.recordings[0] ?? null;
        });
      })
      .catch((cause) => active && setError(cause instanceof Error ? cause : new Error(String(cause))))
      .finally(() => active && setOverviewLoading(false));
    return () => { active = false; };
  }, [api, ownerUserID, refresh]);

  useEffect(() => {
    if (!selectedRecording) {
      setDetail(null);
      return;
    }
    let active = true;
    setDetailLoading(true);
    setError(null);
    setDetail(null);
    api.pipelineDebug.denseRecording(
      ownerUserID, selectedRecording.recording_id, selectedRecording.processing_version,
    ).then((result) => {
      if (!active) return;
      const firstTrack = result.tracks[0];
      const firstObservation = firstTrack?.observations.find((item) => item.gallery_selected)
        ?? firstTrack?.observations[0];
      setDetail(result);
      setSelectedTrackID(firstTrack?.id ?? '');
      setSelectedObservationID(firstObservation?.observation_id ?? '');
    }).catch((cause) => active && setError(cause instanceof Error ? cause : new Error(String(cause))))
      .finally(() => active && setDetailLoading(false));
    return () => { active = false; };
  }, [api, ownerUserID, selectedRecording]);

  const selectedTrack = detail?.tracks.find((track) => track.id === selectedTrackID) ?? null;
  const selectedObservation = selectedTrack?.observations.find(
    (observation) => observation.observation_id === selectedObservationID,
  ) ?? null;
  const scenePeople = useMemo(() => {
    if (!detail || !selectedTrack) return [];
    return (detail.visual_analysis.observations ?? []).flatMap((observation) =>
      observation.people
        .filter((person) => person.person_track_id === selectedTrack.id)
        .map((person) => ({ ...person, timestamp: observation.start_time, observation_id: observation.observation_id })),
    );
  }, [detail, selectedTrack]);

  function chooseTrack(track: PipelineDebugDenseTrack) {
    setSelectedTrackID(track.id);
    setSelectedObservationID(
      track.observations.find((observation) => observation.gallery_selected)?.observation_id
        ?? track.observations[0]?.observation_id
        ?? '',
    );
  }

  return (
    <div style={styles.stack}>
      <section style={styles.card}>
        <div style={styles.cardHeader}>
          <div>
            <p style={styles.eyebrow}>Background worker</p>
            <h3 style={styles.h2}>Dense person tracking</h3>
            <p style={styles.muted}>Queue health and biometric evidence for {ownerEmail}.</p>
          </div>
          <button type="button" onClick={() => setRefresh((value) => value + 1)} style={styles.secondaryButton}>
            Refresh
          </button>
        </div>
        {error ? <ReadableError error={error} /> : null}
        <div style={styles.metrics} aria-live="polite">
          <Metric label="Worker" value={overview?.worker.enabled ? 'Configured' : provider?.enabled ? 'Configured' : 'Disabled'} />
          <Metric label="Processing" value={String(overview?.worker.jobs.processing ?? 0)} />
          <Metric label="Queued" value={String(overview?.worker.jobs.queued ?? 0)} />
          <Metric label="Retryable" value={String(overview?.worker.jobs.retryable_failed ?? 0)} />
          <Metric label="Dead" value={String(overview?.worker.jobs.dead ?? 0)} />
          <Metric label="Completed" value={String(overview?.worker.jobs.completed ?? 0)} />
        </div>
        <dl style={styles.definitionGrid}>
          <dt>Detector</dt><dd>{compactModel(overview?.worker.detector_model || 'Not configured')}</dd>
          <dt>Embedding</dt><dd>{compactModel(overview?.worker.embedding_model || provider?.model || 'Not configured')}</dd>
          <dt>Analysis rate</dt><dd>{overview ? `${fixed(overview.worker.profile.fps)} FPS` : '—'}</dd>
          <dt>Confirmation</dt><dd>{overview ? `${overview.worker.profile.confirmation_detections} detections within ${overview.worker.profile.confirmation_window_frames} frames` : '—'}</dd>
          <dt>Lost / re-ID window</dt><dd>{overview ? `${fixed(overview.worker.profile.lost_timeout_seconds)}s / ${fixed(overview.worker.profile.reidentification_window_seconds)}s` : '—'}</dd>
          <dt>Detection thresholds</dt><dd>{overview ? `${fixed(overview.worker.profile.low_confidence_threshold)} low / ${fixed(overview.worker.profile.high_confidence_threshold)} high` : '—'}</dd>
          <dt>Association thresholds</dt><dd>{overview ? `${fixed(overview.worker.profile.iou_threshold)} IoU / ${fixed(overview.worker.profile.appearance_threshold)} appearance` : '—'}</dd>
          <dt>Gallery limit</dt><dd>{overview ? String(overview.worker.profile.max_gallery_samples) : '—'}</dd>
          <dt>Oldest waiting</dt><dd>{formatDate(overview?.worker.oldest_queued_at)}</dd>
          <dt>Last completed</dt><dd>{formatDate(overview?.worker.last_completed_at)}</dd>
        </dl>
        <div style={styles.biometricNotice}>
          <strong>Biometric debug data</strong>
          <span>Visible only to this configured debug account. Raw vectors are intentionally collapsed until opened.</span>
        </div>
      </section>

      {overviewLoading && !overview ? <section style={styles.card} aria-live="polite">Loading dense worker data…</section> : null}
      {overview && overview.recordings.length === 0 ? (
        <section style={styles.card}>
          <h3 style={styles.h2}>No dense recordings yet</h3>
          <p style={styles.muted}>Upload a video as this debug account and wait for dense_person_tracking to complete.</p>
        </section>
      ) : null}
      {overview && overview.recordings.length > 0 ? (
        <section className="dense-inspector-layout" style={styles.inspectorLayout}>
          <div className="dense-recording-rail" style={styles.recordingRail} aria-label="Dense tracking recordings">
            <p style={styles.eyebrow}>Recent recording versions</p>
            {overview.recordings.map((recording) => {
              const selected = selectedRecording?.recording_id === recording.recording_id
                && selectedRecording.processing_version === recording.processing_version;
              return (
                <button
                  key={`${recording.recording_id}:${recording.processing_version}`}
                  type="button"
                  aria-pressed={selected}
                  onClick={() => setSelectedRecording(recording)}
                  style={{ ...styles.recordingButton, ...(selected ? styles.recordingButtonActive : {}) }}>
                  <span style={styles.recordingTitle}>{recording.file_name}</span>
                  <span style={styles.small}>v{recording.processing_version} · {recording.stage_status}</span>
                  <span style={styles.recordingCounts}>{recording.track_count} tracks · {recording.observation_count} observations</span>
                  {recording.last_error ? <span style={styles.recordingError}>{recording.last_error}</span> : null}
                </button>
              );
            })}
          </div>

          <div style={styles.stack}>
            {detailLoading && !detail ? <section style={styles.card} aria-live="polite">Loading recording evidence…</section> : null}
            {detail ? (
              <DenseRecordingInspector
                api={api}
                ownerUserID={ownerUserID}
                detail={detail}
                selectedTrack={selectedTrack}
                selectedObservation={selectedObservation}
                scenePeople={scenePeople}
                chooseTrack={chooseTrack}
                chooseObservation={setSelectedObservationID}
              />
            ) : null}
          </div>
        </section>
      ) : null}
    </div>
  );
}

function DenseRecordingInspector({
  api, ownerUserID, detail, selectedTrack, selectedObservation, scenePeople, chooseTrack, chooseObservation,
}: {
  api: ApiClient;
  ownerUserID: string;
  detail: PipelineDebugDenseRecordingDetail;
  selectedTrack: PipelineDebugDenseTrack | null;
  selectedObservation: PipelineDebugDenseObservation | null;
  scenePeople: Array<{
    timestamp: number;
    observation_id: string;
    visual_label: string;
    appearance: string;
    position: string;
    action: string;
    person_name?: string;
    face_match_confidence?: number;
  }>;
  chooseTrack: (track: PipelineDebugDenseTrack) => void;
  chooseObservation: (id: string) => void;
}) {
  const recording = detail.recording;
  return (
    <>
      <section style={styles.card}>
        <div style={styles.cardHeader}>
          <div>
            <p style={styles.eyebrow}>Recording evidence</p>
            <h3 style={styles.h2}>{recording.file_name}</h3>
            <p style={styles.small}>{recording.recording_id} · processing version {recording.processing_version}</p>
          </div>
          <span style={{ ...styles.statePill, ...nodeStyle(stageRunState(recording.stage_status)) }}>{recording.stage_status}</span>
        </div>
        <div style={styles.metrics}>
          <Metric label="Tracks" value={String(recording.track_count)} />
          <Metric label="Observations" value={String(recording.observation_count)} />
          <Metric label="Gallery samples" value={String(recording.gallery_count)} />
          <Metric label="Stored vectors" value={String(recording.embedding_count)} />
          <Metric label="Attempts" value={`${recording.attempts} / ${recording.max_attempts}`} />
          <Metric label="Updated" value={formatDate(recording.updated_at)} />
        </div>
        <details style={styles.details}>
          <summary>Stage checkpoint and provenance</summary>
          <pre style={styles.code}>{JSON.stringify({ checkpoint: recording.checkpoint, result_provenance: recording.result_provenance }, null, 2)}</pre>
        </details>
      </section>

      {detail.tracks.length === 0 ? (
        <section style={styles.card}><p style={styles.muted}>The worker completed without detecting a persistent face track.</p></section>
      ) : (
        <section style={styles.card}>
          <p style={styles.eyebrow}>Detected people</p>
          <div style={styles.trackTabs}>
            {detail.tracks.map((track, index) => (
              <button
                key={track.id}
                type="button"
                aria-pressed={track.id === selectedTrack?.id}
                onClick={() => chooseTrack(track)}
                style={{ ...styles.trackButton, ...(track.id === selectedTrack?.id ? styles.trackButtonActive : {}) }}>
                <strong>{track.resolved_person_name || `Person ${index + 1}`}</strong>
                <span>{track.lifecycle_status} · {track.observations.length} frames</span>
              </button>
            ))}
          </div>
        </section>
      )}

      {selectedTrack ? (
        <TrackInspector
          api={api}
          ownerUserID={ownerUserID}
          recording={recording}
          track={selectedTrack}
          observation={selectedObservation}
          scenePeople={scenePeople}
          chooseObservation={chooseObservation}
        />
      ) : null}

      <FusionEvidencePanel
        api={api}
        ownerUserID={ownerUserID}
        recording={recording}
        evidence={detail.fusion_evidence ?? []}
        tracks={detail.tracks}
        chooseTrack={chooseTrack}
      />

      <details style={{ ...styles.card, ...styles.details }}>
        <summary>Complete recording JSON, including every embedding</summary>
        <pre style={styles.code}>{JSON.stringify(detail, null, 2)}</pre>
      </details>
    </>
  );
}

function FusionEvidencePanel({ api, ownerUserID, recording, evidence, tracks, chooseTrack }: {
  api: ApiClient;
  ownerUserID: string;
  recording: PipelineDebugDenseRecording;
  evidence: PipelineDebugFusionEvidence[];
  tracks: PipelineDebugDenseTrack[];
  chooseTrack: (track: PipelineDebugDenseTrack) => void;
}) {
  return (
    <section style={styles.card}>
      <div style={styles.cardHeader}>
        <div>
          <p style={styles.eyebrow}>Labelled voice → dense face</p>
          <h3 style={styles.h2}>Active-speaker fusion evidence</h3>
          <p style={styles.muted}>Every decision is owner-scoped and retained even when the safe result is unknown.</p>
        </div>
        <span style={styles.adminBadge}>{evidence.length} decisions</span>
      </div>
      {evidence.length === 0 ? (
        <p style={styles.muted}>No fusion evidence has been saved for this recording version.</p>
      ) : (
        <div style={styles.fusionList}>
          {evidence.map((item) => {
            const track = tracks.find((candidate) => candidate.id === item.person_track_id);
            const observation = track?.observations.find((candidate) => candidate.gallery_selected)
              ?? track?.observations[0];
            const result = fusionResult(item, track);
            return (
              <article key={item.id} className="fusion-decision-row" style={styles.fusionRow}>
                <FusionFacePreview
                  api={api}
                  ownerUserID={ownerUserID}
                  recording={recording}
                  track={track}
                  observation={observation}
                />
                <div style={styles.fusionBody}>
                  <div style={styles.sectionHeading}>
                    <div>
                      <strong>{item.known_voice_name || item.voice_speaker_profile_id}</strong>
                      <p style={styles.small}>{fixed(item.segment_start_time)}s–{fixed(item.segment_end_time)}s · {item.segment_id}</p>
                    </div>
                    <span style={{ ...styles.statePill, ...fusionResultStyle(result) }}>{result}</span>
                  </div>
                  <div style={styles.fusionMetrics}>
                    <Metric label="Voice confidence" value={percent(item.voice_confidence)} />
                    <Metric label="Active speaker" value={percent(item.active_speaker_score)} />
                    <Metric label="Runner-up" value={percent(item.runner_up_score)} />
                    <Metric label="Decision margin" value={percent(item.decision_margin)} />
                    <Metric label="Temporal coverage" value={percent(item.temporal_coverage)} />
                    <Metric label="Mouth visible" value={percent(item.mouth_visible_coverage)} />
                    <Metric label="Mouth activity" value={percent(item.mouth_activity)} />
                    <Metric label="Supporting segments" value={String(item.supporting_segment_count)} />
                  </div>
                  <dl style={styles.definitionGrid}>
                    <dt>Candidate track</dt><dd>{item.person_track_id || 'No visible candidate'}</dd>
                    <dt>Canonical profile</dt><dd>{item.canonical_person_profile_id || 'Identity unknown'}</dd>
                    <dt>Combined score</dt><dd>{percent(item.combined_score)}</dd>
                    <dt>Conflicts</dt><dd>{item.conflict_reasons.join(', ') || 'None'}</dd>
                  </dl>
                  {track ? (
                    <button type="button" onClick={() => chooseTrack(track)} style={styles.inlineButton}>
                      Inspect candidate track
                    </button>
                  ) : null}
                  <details style={styles.details}>
                    <summary>Raw evidence JSON</summary>
                    <pre style={styles.code}>{JSON.stringify(item, null, 2)}</pre>
                  </details>
                </div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}

function FusionFacePreview({ api, ownerUserID, recording, track, observation }: {
  api: ApiClient;
  ownerUserID: string;
  recording: PipelineDebugDenseRecording;
  track?: PipelineDebugDenseTrack;
  observation?: PipelineDebugDenseObservation;
}) {
  const [url, setURL] = useState('');
  useEffect(() => {
    if (!track || !observation) return;
    let active = true;
    let objectURL = '';
    api.pipelineDebug.denseFace(
      ownerUserID, recording.recording_id, track.id,
      observation.observation_id, recording.processing_version,
    ).then((blob) => {
      if (!active) return;
      objectURL = URL.createObjectURL(blob);
      setURL(objectURL);
    }).catch(() => undefined);
    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [api, observation, ownerUserID, recording.processing_version, recording.recording_id, track]);
  return (
    <div style={styles.fusionFace}>
      {url ? <img src={url} alt="Candidate face" style={styles.fusionFaceImage} /> : (
        <span style={styles.small}>{track ? 'Face preview unavailable' : 'Off-screen voice'}</span>
      )}
    </div>
  );
}

function TrackInspector({ api, ownerUserID, recording, track, observation, scenePeople, chooseObservation }: {
  api: ApiClient;
  ownerUserID: string;
  recording: PipelineDebugDenseRecording;
  track: PipelineDebugDenseTrack;
  observation: PipelineDebugDenseObservation | null;
  scenePeople: Array<{ timestamp: number; observation_id: string; visual_label: string; appearance: string; position: string; action: string; person_name?: string; face_match_confidence?: number }>;
  chooseObservation: (id: string) => void;
}) {
  const metrics = track.metrics;
  return (
    <section style={styles.card}>
      <div style={styles.cardHeader}>
        <div>
          <p style={styles.eyebrow}>Track detail</p>
          <h3 style={styles.h2}>{track.resolved_person_name || track.temporary_visual_label}</h3>
          <p style={styles.small}>{track.id}</p>
        </div>
        <span style={track.resolved_person_profile_id ? styles.readyBadge : styles.offBadge}>
          {track.resolved_person_profile_id ? `${track.resolved_person_status} identity` : 'Unknown identity'}
        </span>
      </div>
      <div style={styles.metrics}>
        <Metric label="Tracking confidence" value={percent(track.tracking_confidence)} />
        <Metric label="Duration" value={`${fixed(metrics.duration_seconds)} s`} />
        <Metric label="Observation rate" value={`${fixed(metrics.observations_per_second)} / s`} />
        <Metric label="Detection min / mean / max" value={`${fixed(metrics.detection_minimum)} / ${fixed(metrics.detection_mean)} / ${fixed(metrics.detection_maximum)}`} />
        <Metric label="Quality mean / max" value={`${fixed(track.quality.mean)} / ${fixed(track.quality.maximum)}`} />
        <Metric label="Gallery coverage" value={percent(metrics.gallery_coverage)} />
        <Metric label="Mouth visible" value={percent(metrics.mouth_visible_coverage)} />
        <Metric label="Mouth activity mean" value={fixed(metrics.mouth_activity_mean)} />
        <Metric label="Largest frame gap" value={`${fixed(metrics.maximum_observation_gap_seconds)} s`} />
        <Metric label="Box continuity IoU" value={fixed(metrics.mean_consecutive_box_iou)} />
        <Metric label="Embedding norm" value={fixed(metrics.embedding_norm_mean, 4)} />
        <Metric label="Stored embeddings" value={`${metrics.embedding_count} × ${metrics.embedding_dimensions || '—'}D`} />
        <Metric label="Embedding cosine min / mean" value={`${optionalFixed(metrics.embedding_cosine_minimum)} / ${optionalFixed(metrics.embedding_cosine_mean)}`} />
      </div>

      <div className="dense-detail-columns" style={styles.detailColumns}>
        <FacePreview api={api} ownerUserID={ownerUserID} recording={recording} track={track} observation={observation} />
        <div style={styles.plainSection}>
          <h4 style={styles.h3}>Identity and person description</h4>
          <dl style={styles.definitionGrid}>
            <dt>Profile</dt><dd>{track.resolved_person_profile_id || 'No profile matched'}</dd>
            <dt>Provider track</dt><dd>{track.provider_track_reference}</dd>
            <dt>Timeline</dt><dd>{fixed(track.start_time)}s–{fixed(track.end_time)}s</dd>
            <dt>Frames</dt><dd>{track.first_frame}–{track.last_frame}</dd>
            <dt>Pose distribution</dt><dd>{Object.entries(metrics.pose_buckets).map(([key, value]) => `${key}: ${value}`).join(' · ') || 'None'}</dd>
          </dl>
          {scenePeople.length ? scenePeople.map((person) => (
            <div key={`${person.observation_id}:${person.visual_label}`} style={styles.scenePerson}>
              <strong>{person.person_name || person.visual_label} at {fixed(person.timestamp)}s</strong>
              <span>{person.appearance || 'No appearance description'}</span>
              <span>{person.action || 'No action'} · {person.position || 'position unavailable'}</span>
              {person.face_match_confidence !== undefined ? <span>Face match {percent(person.face_match_confidence)}</span> : null}
            </div>
          )) : <p style={styles.muted}>No sampled scene description has been mapped to this dense track yet.</p>}
        </div>
      </div>

      <div style={styles.plainSection}>
        <div style={styles.sectionHeading}>
          <div>
            <h4 style={styles.h3}>Observation timeline</h4>
            <p style={styles.small}>Select a row to inspect its crop, landmarks, pose, quality, and vector.</p>
          </div>
          <span style={styles.small}>{track.observations.length} total</span>
        </div>
        <div style={styles.tableScroll}>
          <table className="dense-table" style={styles.table}>
            <thead><tr><th>Time</th><th>Frame</th><th>Detection</th><th>Quality</th><th>Pose</th><th>Mouth</th><th>Gallery</th><th>Vector</th></tr></thead>
            <tbody>
              {track.observations.map((item) => (
                <tr
                  key={item.observation_id}
                  style={item.observation_id === observation?.observation_id ? styles.tableRowActive : styles.tableRow}>
                  <td>
                    <button
                      type="button"
                      aria-label={`Inspect observation at ${fixed(item.timestamp)} seconds`}
                      onClick={() => chooseObservation(item.observation_id)}
                      style={styles.rowButton}>
                      {fixed(item.timestamp)}s
                    </button>
                  </td>
                  <td>{item.frame_index}</td><td>{fixed(item.detection_score)}</td><td>{fixed(item.quality.score)}</td>
                  <td>{item.pose.bucket}</td><td>{fixed(item.mouth_activity)}</td>
                  <td>{item.gallery_selected ? 'Selected' : '—'}</td><td>{item.embedding_dimensions || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <details style={styles.details}>
        <summary>Complete track JSON</summary>
        <pre style={styles.code}>{JSON.stringify(track, null, 2)}</pre>
      </details>
    </section>
  );
}

function FacePreview({ api, ownerUserID, recording, track, observation }: {
  api: ApiClient;
  ownerUserID: string;
  recording: PipelineDebugDenseRecording;
  track: PipelineDebugDenseTrack;
  observation: PipelineDebugDenseObservation | null;
}) {
  const [url, setURL] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    if (!observation) {
      setURL('');
      return;
    }
    let active = true;
    let objectURL = '';
    setURL('');
    setError('');
    api.pipelineDebug.denseFace(
      ownerUserID, recording.recording_id, track.id,
      observation.observation_id, recording.processing_version,
    ).then((blob) => {
      if (!active) return;
      objectURL = URL.createObjectURL(blob);
      setURL(objectURL);
    }).catch((cause) => active && setError(cause instanceof Error ? cause.message : String(cause)));
    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [api, observation, ownerUserID, recording.processing_version, recording.recording_id, track.id]);

  return (
    <div style={styles.facePanel} aria-live="polite">
      <div style={styles.faceImageFrame}>
        {url ? <img src={url} alt={`Detected face at ${fixed(observation?.timestamp ?? 0)} seconds`} style={styles.faceImage} /> : null}
        {!url && !error ? <span style={styles.muted}>{observation ? 'Extracting face crop…' : 'Select an observation'}</span> : null}
        {error ? <span style={styles.recordingError}>{error}</span> : null}
      </div>
      {observation ? (
        <>
          <h4 style={styles.h3}>Observation at {fixed(observation.timestamp)}s</h4>
          <dl style={styles.definitionGrid}>
            <dt>ID</dt><dd>{observation.observation_id}</dd>
            <dt>Box</dt><dd>x {observation.box.x}, y {observation.box.y}, {observation.box.width}×{observation.box.height}</dd>
            <dt>Pose</dt><dd>{observation.pose.bucket} · yaw {fixed(observation.pose.yaw)} · pitch {fixed(observation.pose.pitch)} · roll {fixed(observation.pose.roll)}</dd>
            <dt>Quality gate</dt><dd>{observation.quality.usable ? 'Usable' : `Rejected: ${observation.quality.reasons.join(', ') || 'unspecified'}`}</dd>
            <dt>Landmarks</dt><dd>{observation.landmarks.map((point) => `(${point.map((value) => fixed(value)).join(', ')})`).join(' ')}</dd>
            <dt>Gallery</dt><dd>{observation.gallery_selected ? 'Selected for matching' : 'Tracking only'}</dd>
          </dl>
          <details style={styles.details}>
            <summary>Embedding vector ({observation.embedding_dimensions || 0} dimensions)</summary>
            <pre style={styles.code}>{JSON.stringify({
              model: observation.embedding_model,
              reference: observation.embedding_reference,
              dimensions: observation.embedding_dimensions,
              embedding: observation.embedding,
            }, null, 2)}</pre>
          </details>
          <details style={styles.details}>
            <summary>Complete observation JSON</summary>
            <pre style={styles.code}>{JSON.stringify(observation, null, 2)}</pre>
          </details>
        </>
      ) : null}
    </div>
  );
}

function TestPanel({
  page, file, setFile, metadata, setMetadata, provider, running, execute,
}: {
  page: TestPage;
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
          <p style={styles.small}>This isolated upload test does not read or modify the selected owner’s persisted evidence.</p>
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

function stageRunState(status: string): RunState {
  if (status === 'completed') return 'success';
  if (status === 'processing') return 'running';
  if (status === 'dead' || status === 'retryable_failed') return 'error';
  return 'idle';
}

function fixed(value: number, digits = 2) {
  return Number.isFinite(value) ? value.toFixed(digits) : '—';
}

function optionalFixed(value?: number) {
  return value === undefined ? '—' : fixed(value, 4);
}

function percent(value: number) {
  return Number.isFinite(value) ? `${(value * 100).toFixed(1)}%` : '—';
}

function fusionResult(item: PipelineDebugFusionEvidence, track?: PipelineDebugDenseTrack) {
  if (item.conflict_reasons.includes('off_screen')) return 'off-screen';
  if (item.decision === 'accepted' && track?.resolved_person_profile_id === item.canonical_person_profile_id) return 'linked';
  return item.decision;
}

function fusionResultStyle(result: string): CSSProperties {
  if (result === 'linked') return { color: colors.green, borderColor: '#245c38', background: colors.greenSoft };
  if (result === 'rejected') return { color: colors.red, borderColor: '#713042', background: colors.redSoft };
  if (result === 'off-screen') return { color: colors.cyan, borderColor: '#155e75', background: '#0b2530' };
  return { color: colors.amber, borderColor: '#725f13', background: colors.amberSoft };
}

function formatDate(value?: string) {
  if (!value) return 'None';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
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
  ownerControls: { display: 'flex', alignItems: 'flex-end', justifyContent: 'flex-end', flexWrap: 'wrap', gap: 10 },
  ownerLabel: { display: 'grid', gap: 5, color: colors.muted, fontSize: 10, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '.06em' },
  ownerSelect: { minWidth: 260, minHeight: 40, padding: '8px 34px 8px 10px', border: `1px solid ${colors.border}`, borderRadius: 8, background: colors.panel, color: colors.text, fontSize: 12 },
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
  runList: { display: 'grid', gap: 12 },
  runItem: { minWidth: 0, paddingTop: 16, borderTop: `1px solid ${colors.border}` },
  stageGrid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 8 },
  stageItem: { minWidth: 0, padding: 10, border: `1px solid ${colors.border}`, borderRadius: 8, background: colors.page, color: colors.muted, fontSize: 11 },
  stageSummary: { display: 'flex', justifyContent: 'space-between', gap: 10, cursor: 'pointer', textTransform: 'capitalize' },
	inspectorLayout: { display: 'grid', gridTemplateColumns: 'minmax(230px, 290px) minmax(0, 1fr)', gap: 16, alignItems: 'start' },
	recordingRail: { display: 'grid', gap: 8, position: 'sticky', top: 0, maxHeight: 'calc(100vh - 80px)', overflowY: 'auto', background: colors.panel, border: `1px solid ${colors.border}`, borderRadius: 12, padding: 14 },
	recordingButton: { display: 'grid', gap: 4, minWidth: 0, padding: 11, textAlign: 'left', color: colors.muted, background: colors.page, border: `1px solid ${colors.border}`, borderRadius: 8, cursor: 'pointer' },
	recordingButtonActive: { color: colors.text, borderColor: '#6353a3', background: colors.purpleSoft },
	recordingTitle: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: 700 },
	recordingCounts: { color: colors.text, fontSize: 11 },
	recordingError: { color: colors.red, fontSize: 11, lineHeight: 1.4, overflowWrap: 'anywhere' },
	secondaryButton: { minHeight: 40, border: `1px solid ${colors.border}`, borderRadius: 8, background: colors.raised, color: colors.text, padding: '8px 12px', cursor: 'pointer', fontWeight: 700 },
	biometricNotice: { display: 'flex', flexWrap: 'wrap', gap: '4px 10px', marginTop: 16, paddingTop: 14, borderTop: `1px solid ${colors.border}`, color: colors.amber, fontSize: 11 },
	definitionGrid: { display: 'grid', gridTemplateColumns: 'minmax(95px, auto) minmax(0, 1fr)', gap: '6px 12px', margin: 0, color: colors.muted, fontSize: 11, overflowWrap: 'anywhere' },
	trackTabs: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: 8, marginTop: 12 },
	trackButton: { minHeight: 52, display: 'grid', gap: 3, textAlign: 'left', padding: '9px 11px', border: `1px solid ${colors.border}`, borderRadius: 8, background: colors.page, color: colors.muted, cursor: 'pointer', fontSize: 11 },
	trackButtonActive: { color: colors.text, borderColor: '#6353a3', background: colors.purpleSoft },
	detailColumns: { display: 'grid', gridTemplateColumns: 'minmax(260px, .8fr) minmax(280px, 1.2fr)', gap: 20, alignItems: 'start', marginTop: 20 },
	plainSection: { minWidth: 0, display: 'grid', gap: 12, paddingTop: 18, marginTop: 18, borderTop: `1px solid ${colors.border}` },
	sectionHeading: { display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' },
	scenePerson: { display: 'grid', gap: 4, borderLeft: `2px solid ${colors.purple}`, paddingLeft: 10, color: colors.muted, fontSize: 11, lineHeight: 1.45 },
	facePanel: { minWidth: 0, display: 'grid', gap: 12 },
	faceImageFrame: { minHeight: 220, display: 'grid', placeItems: 'center', overflow: 'hidden', background: '#070708', border: `1px solid ${colors.border}`, borderRadius: 8, padding: 10, textAlign: 'center' },
	faceImage: { display: 'block', width: '100%', height: 260, objectFit: 'contain' },
	fusionList: { display: 'grid' },
	fusionRow: { minWidth: 0, display: 'grid', gridTemplateColumns: '120px minmax(0, 1fr)', gap: 16, padding: '18px 0', borderTop: `1px solid ${colors.border}` },
	fusionFace: { width: 120, height: 120, display: 'grid', placeItems: 'center', overflow: 'hidden', background: colors.page, border: `1px solid ${colors.border}`, borderRadius: 8, textAlign: 'center', padding: 8 },
	fusionFaceImage: { width: '100%', height: '100%', objectFit: 'contain' },
	fusionBody: { minWidth: 0, display: 'grid', gap: 12 },
	fusionMetrics: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(125px, 1fr))', gap: 8 },
	inlineButton: { justifySelf: 'start', minHeight: 40, padding: '8px 11px', border: `1px solid ${colors.border}`, borderRadius: 8, background: colors.raised, color: colors.text, cursor: 'pointer', fontWeight: 700 },
	tableScroll: { maxHeight: 430, overflow: 'auto', border: `1px solid ${colors.border}`, borderRadius: 8 },
	table: { width: '100%', borderCollapse: 'collapse', color: colors.muted, fontSize: 11, textAlign: 'left' },
	tableRow: { background: colors.page, borderTop: `1px solid ${colors.border}` },
	tableRowActive: { background: colors.purpleSoft, color: colors.text, borderTop: '1px solid #493a84' },
	rowButton: { minWidth: 60, minHeight: 36, padding: 0, border: 0, background: 'transparent', color: 'inherit', cursor: 'pointer', textAlign: 'left', fontWeight: 700 },
	details: { color: colors.muted, fontSize: 11, cursor: 'pointer' },
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

const responsiveStyles = `
  .pipeline-debug-shell,
  .pipeline-debug-shell * {
    scrollbar-width: none;
    -ms-overflow-style: none;
  }
  .pipeline-debug-shell::-webkit-scrollbar,
  .pipeline-debug-shell *::-webkit-scrollbar {
    display: none;
    width: 0;
    height: 0;
  }
  .dense-table th,
  .dense-table td {
    padding: 8px 10px;
    white-space: nowrap;
  }
  .dense-table thead {
    position: sticky;
    top: 0;
    z-index: 1;
    background: ${colors.raised};
  }
  @media (max-width: 900px) {
    .dense-inspector-layout,
    .dense-detail-columns {
      grid-template-columns: minmax(0, 1fr) !important;
    }
    .dense-recording-rail {
      position: static !important;
      max-height: none !important;
    }
  }
  @media (max-width: 720px) {
    .pipeline-debug-shell {
      grid-template-columns: minmax(0, 1fr) !important;
      height: auto !important;
    }
    .pipeline-debug-sidebar {
      position: static !important;
      border-right: 0 !important;
      border-bottom: 1px solid ${colors.border};
    }
    .pipeline-debug-content {
      padding: 24px 16px 64px !important;
    }
    .pipeline-debug-header,
    .pipeline-debug-owner-controls {
      align-items: stretch !important;
      flex-direction: column !important;
    }
    .pipeline-debug-owner-controls select {
      min-width: 0 !important;
      width: 100% !important;
    }
    .fusion-decision-row {
      grid-template-columns: minmax(0, 1fr) !important;
    }
  }
`;
