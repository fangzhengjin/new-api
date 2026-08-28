/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

/** Describes one upstream model and the public model/mapping recommended for it. */
export type ChannelDiscoveryChoice = {
  source: string
  model: string
  target: string
  recommended: boolean
}

/** Identifies an existing channel that matches a discovered Base URL. */
export type ChannelDiscoveryMatch = {
  id: number
  name: string
}

/** Contains one redacted connection block and its discovered configuration options. */
export type ChannelDiscoveryConnection = {
  block_index: number
  base_url: string
  suggested_name: string
  usable: boolean
  models: string[]
  choices: ChannelDiscoveryChoice[]
  usable_key_indexes?: number[] | null
  rejected_key_count: number
  matches: ChannelDiscoveryMatch[]
  models_path?: string
  error_message?: string
}

/** Maps each successful inference protocol to its same-origin upstream path. */
export type ChannelDiscoveryProbe = Record<string, string>

/** Identifies the exact source, key, Base URL, and model used by one probe. */
export type ChannelDiscoveryProbeInput = {
  text: string
  block_index: number
  key_index: number
  base_url: string
  model: string
  models_path?: string
}

/** Contains all administrator choices required to preview a create or update. */
export type ChannelDiscoveryDraft = {
  operation: 'create' | 'update'
  block_index: number
  channel_id?: number
  base_url: string
  accepted_key_indexes: number[]
  selected_models: string[]
  mapping: Record<string, string>
  routes: Record<string, string>
  name?: string
  groups?: string[]
  tag?: string
  priority?: number
  enabled: boolean
  key_mode?: 'append' | 'replace'
  sync_configuration: boolean
  reenable: boolean
}

/** Describes the immutable server-side write preview and concurrency hashes. */
export type ChannelDiscoveryPreview = {
  operation: 'create' | 'update'
  name: string
  type_name: string
  status: number
  base_url: string
  groups: string[]
  tag: string
  priority: number
  models: string[]
  mapping: Record<string, string>
  routes: Array<{
    protocol: string
    incoming_path: string
    upstream_path: string
  }>
  key_count: number
  changes: string[]
  snapshot_hash?: string
  preview_hash: string
}

/** Binds a preview draft to the raw source text that supplied its credentials. */
export type ChannelDiscoveryPreviewInput = {
  text: string
  draft: ChannelDiscoveryDraft
}

/** Contains selectable normalization diffs for one snapshot-bound channel. */
export type ChannelNormalizationItem = {
  channel_id: number
  snapshot_hash: string
  add_models?: string[]
  remove_models?: string[]
  mapping_set?: Record<string, string>
  mapping_remove?: string[]
  mapping_warnings?: Record<string, string>
  sort_changed?: boolean
}

/** Contains the compact result persisted by a completed normalization scan. */
export type ChannelNormalizationResult = {
  summary: {
    scanned: number
    changed: number
    skipped: number
    failed: number
  }
  items?: ChannelNormalizationItem[]
  failures?: Array<{
    channel_id: number
    error_message: string
  }>
  applied_at?: number
}

/** Describes the durable background task and its resumable scan progress. */
export type ChannelNormalizationTask = {
  task_id: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  state?: {
    processed?: number
    total?: number
    current_channel_id?: number
  }
  result?: ChannelNormalizationResult
  error?: string
}

/** Combines a normalization task with display-only current channel metadata. */
export type ChannelNormalizationData = {
  task: ChannelNormalizationTask
  channels: Record<
    string,
    {
      name: string
      models: string[]
      model_mapping: Record<string, string>
    }
  >
}

/** Contains the exact per-channel subitems selected for atomic application. */
export type ChannelNormalizationSelection = {
  channel_id: number
  add_models: string[]
  remove_models: string[]
  mapping_set: Record<string, string>
  mapping_remove: string[]
  sort_models: boolean
}

type ApiResponse<T> = {
  success: boolean
  message?: string
  code?: string
  data?: T
}

function requireData<T>(response: ApiResponse<T>): T {
  if (!response.success || response.data === undefined) {
    throw Object.assign(new Error(response.message || 'Request failed'), {
      code: response.code,
    })
  }
  return response.data
}

const setupRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
}

/**
 * Identifies image models whose route can be inferred without a billable probe.
 * @param model - Upstream or exposed model ID to classify.
 * @returns Whether the model belongs to a known image family.
 */
export function isChannelDiscoveryImageModel(model: string): boolean {
  return (
    model.startsWith('gpt-image-') ||
    model.startsWith('imagen-') ||
    model.startsWith('dall-e-')
  )
}

/**
 * Chooses a representative probe model only from models the user selected.
 * @param models - Currently selected exposed model IDs.
 * @returns Luna when selected, otherwise the first non-image model or fallback.
 */
export function selectChannelDiscoveryProbeModel(models: string[]): string {
  return (
    models.find((model) => model === 'gpt-5.6-luna') ??
    models.find((model) => !isChannelDiscoveryImageModel(model)) ??
    models[0] ??
    ''
  )
}

/**
 * Checks whether a discovery response still belongs to the current source text.
 * @param requestText - Source text captured when the request started.
 * @param currentText - Source text currently shown in the form.
 * @returns Whether the response is still current.
 */
export function isCurrentChannelDiscoveryText(
  requestText: string,
  currentText: string
): boolean {
  return currentText.trim() === requestText
}

/**
 * Checks whether a protocol-probe response still targets the active selection.
 * @param request - Probe identity captured when the request started.
 * @param current - Probe identity derived from the current form state.
 * @returns Whether the response may update the current route state.
 */
export function isCurrentChannelDiscoveryProbe(
  request: ChannelDiscoveryProbeInput,
  current: ChannelDiscoveryProbeInput
): boolean {
  return (
    request.text === current.text &&
    request.block_index === current.block_index &&
    request.key_index === current.key_index &&
    request.base_url === current.base_url &&
    request.model === current.model &&
    request.models_path === current.models_path
  )
}

/**
 * Checks whether a preview response still represents the current editable draft.
 * @param request - Preview input captured when the request started.
 * @param current - Preview input derived from the current form state.
 * @returns Whether the response may open the current review dialog.
 */
export function isCurrentChannelDiscoveryPreview(
  request: ChannelDiscoveryPreviewInput,
  current: ChannelDiscoveryPreviewInput
): boolean {
  const requestDraft = {
    ...request.draft,
    base_url: request.draft.base_url.trim(),
    name: request.draft.name?.trim(),
    tag: request.draft.tag?.trim(),
  }
  const currentDraft = {
    ...current.draft,
    base_url: current.draft.base_url.trim(),
    name: current.draft.name?.trim(),
    tag: current.draft.tag?.trim(),
  }
  return (
    request.text.trim() === current.text.trim() &&
    JSON.stringify(requestDraft) === JSON.stringify(currentDraft)
  )
}

/**
 * Sends raw connection blocks for server-side validation.
 * @param text - Pasted URLs and API keys grouped by connection.
 * @returns Redacted discovery candidates; credentials are never returned.
 */
export async function discoverChannels(
  text: string
): Promise<ChannelDiscoveryConnection[]> {
  const response = await api.post<ApiResponse<ChannelDiscoveryConnection[]>>(
    '/api/channel/discovery',
    { text },
    setupRequestConfig
  )
  return requireData(response.data)
}

/**
 * Probes inference protocols for the selected discovered key and model.
 * @param input - Exact source, key, Base URL, and model probe identity.
 * @returns Successful protocol-to-path mappings.
 */
export async function probeChannelDiscovery(
  input: ChannelDiscoveryProbeInput
): Promise<ChannelDiscoveryProbe> {
  const response = await api.post<ApiResponse<ChannelDiscoveryProbe>>(
    '/api/channel/discovery/probe',
    input,
    setupRequestConfig
  )
  return requireData(response.data)
}

/**
 * Validates a channel draft server-side against current upstream state.
 * @param input - Raw source text and the editable channel draft.
 * @returns The immutable write preview and its concurrency hashes.
 */
export async function previewChannelDiscovery(
  input: ChannelDiscoveryPreviewInput
): Promise<ChannelDiscoveryPreview> {
  const response = await api.post<ApiResponse<ChannelDiscoveryPreview>>(
    '/api/channel/discovery/preview',
    input,
    setupRequestConfig
  )
  return requireData(response.data)
}

/**
 * Revalidates and applies a previously reviewed channel preview.
 * @param input - Reviewed source, draft, preview hash, and snapshot hash.
 * @returns The created or updated channel ID.
 */
export async function applyChannelDiscovery(input: {
  text: string
  draft: ChannelDiscoveryDraft
  preview_hash: string
  snapshot_hash: string
}): Promise<{ id: number }> {
  const response = await api.post<ApiResponse<{ id: number }>>(
    '/api/channel/discovery/apply',
    input,
    setupRequestConfig
  )
  return requireData(response.data)
}

/**
 * Loads the active or most recent normalization task.
 * @returns Task data with current channel metadata, or null before the first scan.
 */
export async function getCurrentChannelNormalization(): Promise<ChannelNormalizationData | null> {
  const response = await api.get<ApiResponse<ChannelNormalizationData | null>>(
    '/api/channel/normalization/current',
    setupRequestConfig
  )
  return requireData(response.data)
}

/**
 * Starts or resumes the single active normalization scan.
 * @param includeDisabled - Whether disabled channels participate in the scan.
 * @returns The durable task and current channel metadata.
 */
export async function startChannelNormalization(
  includeDisabled: boolean
): Promise<ChannelNormalizationData> {
  const response = await api.post<ApiResponse<ChannelNormalizationData>>(
    '/api/channel/normalization/scan',
    { include_disabled: includeDisabled },
    setupRequestConfig
  )
  return requireData(response.data)
}

/**
 * Atomically applies reviewed selections for one completed task.
 * @param input - Completed task ID and selected per-channel subitems.
 * @returns The number of channels whose persisted configuration changed.
 */
export async function applyChannelNormalization(input: {
  task_id: string
  channels: ChannelNormalizationSelection[]
}): Promise<{ updated: number }> {
  const response = await api.post<ApiResponse<{ updated: number }>>(
    '/api/channel/normalization/apply',
    input,
    setupRequestConfig
  )
  return requireData(response.data)
}
