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
import type {
  ChannelNormalizationItem,
  ChannelNormalizationSelection,
} from '../channel-setup'

/** Contains the current models and mappings used to reconcile one preview item. */
export type ChannelNormalizationMetadata = {
  models: string[]
  model_mapping: Record<string, string>
}

/**
 * Creates safe defaults with additions and sorting enabled but removals disabled.
 * @param item - Server-approved selectable diffs for one channel.
 * @param metadata - Current channel models and mappings.
 * @returns A dependency-consistent initial selection.
 */
export function createDefaultNormalizationSelection(
  item: ChannelNormalizationItem,
  metadata: ChannelNormalizationMetadata
): ChannelNormalizationSelection {
  const addModels = [...(item.add_models ?? [])]
  const mappingSet: Record<string, string> = {}
  for (const [source, target] of Object.entries(item.mapping_set ?? {})) {
    if (metadata.model_mapping[source] === undefined) {
      mappingSet[source] = target
    }
  }
  return {
    channel_id: item.channel_id,
    add_models: addModels,
    remove_models: [],
    mapping_set: mappingSet,
    mapping_remove: [],
    sort_models: item.sort_changed === true || addModels.length > 0,
  }
}

/**
 * Selects every server-approved change for one channel.
 * @param item - Server-approved selectable diffs for one channel.
 * @param metadata - Current channel models and mappings.
 * @returns A dependency-consistent selection containing every previewed change.
 */
export function createFullNormalizationSelection(
  item: ChannelNormalizationItem,
  metadata: ChannelNormalizationMetadata
): ChannelNormalizationSelection {
  return reconcileNormalizationSelection(item, metadata, {
    channel_id: item.channel_id,
    add_models: [...(item.add_models ?? [])],
    remove_models: [...(item.remove_models ?? [])],
    mapping_set: { ...item.mapping_set },
    mapping_remove: [...(item.mapping_remove ?? [])],
    sort_models:
      item.sort_changed === true ||
      (item.add_models?.length ?? 0) > 0 ||
      (item.remove_models?.length ?? 0) > 0,
  })
}

/**
 * Clears every selected change for one channel.
 * @param item - Preview item whose channel identifier is retained.
 * @returns An empty selection that requests no mutation.
 */
export function createEmptyNormalizationSelection(
  item: ChannelNormalizationItem
): ChannelNormalizationSelection {
  return {
    channel_id: item.channel_id,
    add_models: [],
    remove_models: [],
    mapping_set: {},
    mapping_remove: [],
    sort_models: false,
  }
}

/**
 * Reconciles model/mapping dependencies against current metadata.
 * @param item - Server-approved selectable diffs for one channel.
 * @param metadata - Current channel models and mappings.
 * @param selection - Administrator choices to reconcile.
 * @returns A selection with required mappings and model-change sorting enabled.
 */
export function reconcileNormalizationSelection(
  item: ChannelNormalizationItem,
  metadata: ChannelNormalizationMetadata,
  selection: ChannelNormalizationSelection
): ChannelNormalizationSelection {
  const finalModels = new Set(metadata.models)
  for (const model of selection.add_models) finalModels.add(model)
  for (const model of selection.remove_models) finalModels.delete(model)

  const mappingSet = Object.fromEntries(
    Object.entries(selection.mapping_set).filter(([source]) =>
      finalModels.has(source)
    )
  )
  const addedModels = new Set(selection.add_models)
  for (const [source, target] of Object.entries(item.mapping_set ?? {})) {
    if (
      addedModels.has(source) &&
      metadata.model_mapping[source] === undefined
    ) {
      mappingSet[source] ??= target
    }
  }
  const mappingRemove = new Set(selection.mapping_remove)
  for (const source of item.mapping_remove ?? []) {
    if (
      !finalModels.has(source) &&
      metadata.model_mapping[source] !== undefined
    ) {
      mappingRemove.add(source)
    }
  }

  // Sorting is derived from selected model changes so the UI cannot save an
  // appended/filtered list that only becomes visibly unsorted on the next scan.
  const modelChangesSelected =
    selection.add_models.length > 0 || selection.remove_models.length > 0

  return {
    ...selection,
    mapping_set: mappingSet,
    mapping_remove: [...mappingRemove].sort(),
    sort_models:
      modelChangesSelected ||
      (item.sort_changed === true && selection.sort_models),
  }
}

/**
 * Checks whether a channel selection contains at least one requested change.
 * @param selection - Per-channel subitems selected by the administrator.
 * @returns Whether applying the selection would request any operation.
 */
export function hasNormalizationChanges(
  selection: ChannelNormalizationSelection
): boolean {
  return (
    selection.add_models.length > 0 ||
    selection.remove_models.length > 0 ||
    Object.keys(selection.mapping_set).length > 0 ||
    selection.mapping_remove.length > 0 ||
    selection.sort_models
  )
}

/**
 * Counts selected model, mapping, and sorting operations.
 * @param selection - Per-channel subitems selected by the administrator.
 * @returns The number of independently previewed operations selected.
 */
export function countNormalizationChanges(
  selection: ChannelNormalizationSelection
): number {
  return (
    selection.add_models.length +
    selection.remove_models.length +
    Object.keys(selection.mapping_set).length +
    selection.mapping_remove.length +
    Number(selection.sort_models)
  )
}

/**
 * Checks whether every selected mapping has a non-empty target.
 * @param selection - Per-channel subitems selected by the administrator.
 * @returns Whether all editable mapping targets are valid.
 */
export function isNormalizationSelectionValid(
  selection: ChannelNormalizationSelection
): boolean {
  return Object.values(selection.mapping_set).every(
    (target) => target.trim() !== ''
  )
}
