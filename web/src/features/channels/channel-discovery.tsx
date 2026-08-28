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
import { zodResolver } from '@hookform/resolvers/zod'
import { AiScanIcon, ArrowLeft01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { PageFooterPortal, SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Spinner } from '@/components/ui/spinner'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import { getGroups } from './api'
import {
  applyChannelDiscovery,
  discoverChannels,
  isChannelDiscoveryImageModel,
  isCurrentChannelDiscoveryPreview,
  isCurrentChannelDiscoveryProbe,
  isCurrentChannelDiscoveryText,
  previewChannelDiscovery,
  probeChannelDiscovery,
  selectChannelDiscoveryProbeModel,
  type ChannelDiscoveryChoice,
  type ChannelDiscoveryConnection,
  type ChannelDiscoveryDraft,
  type ChannelDiscoveryPreview,
  type ChannelDiscoveryPreviewInput,
  type ChannelDiscoveryProbeInput,
} from './channel-setup'
import { ChannelDiscoveryPreviewDialog } from './components/channel-discovery-preview-dialog'
import { ChannelDiscoverySourcePanel } from './components/channel-discovery-source-panel'
import { ChannelDiscoveryWorkspace } from './components/channel-discovery-workspace'
import { channelsQueryKeys } from './lib'
import {
  discoveryFormSchema,
  channelSetupLayoutClasses,
  type DiscoveryFormValues,
} from './lib/channel-discovery-form'

type DiscoveryReview = {
  text: string
  draft: ChannelDiscoveryDraft
  preview: ChannelDiscoveryPreview
}

type DiscoveryEditorState = {
  formValues: Omit<DiscoveryFormValues, 'text'>
  selectedModels: string[]
  mapping: Record<string, string>
  routes: Record<string, string>
  acceptedKeyIndexes: number[]
  groups: string[]
  probeModel: string
  lastProbeInput: ChannelDiscoveryProbeInput | null
}

const discoveryDefaultValues: DiscoveryFormValues = {
  text: '',
  operation: 'create',
  channelId: 0,
  baseUrl: '',
  name: '',
  tag: '',
  priority: 0,
  keyMode: 'append',
  syncConfiguration: true,
  enabled: true,
  reenable: false,
}

/**
 * Coordinates discovery, probing, preview, and apply requests.
 * @returns The complete smart-recognition page.
 */
export function ChannelDiscovery() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canPreview = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const canApply = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const form = useForm<DiscoveryFormValues>({
    resolver: zodResolver(discoveryFormSchema),
    defaultValues: discoveryDefaultValues,
  })
  const [connections, setConnections] = useState<ChannelDiscoveryConnection[]>(
    []
  )
  const [selectedBlockIndex, setSelectedBlockIndex] = useState<number | null>(
    null
  )
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [mapping, setMapping] = useState<Record<string, string>>({})
  const [routes, setRoutes] = useState<Record<string, string>>({})
  const [acceptedKeyIndexes, setAcceptedKeyIndexes] = useState<number[]>([])
  const [groups, setGroups] = useState<string[]>(['default'])
  const [probeModel, setProbeModel] = useState('')
  const [lastProbeInput, setLastProbeInput] =
    useState<ChannelDiscoveryProbeInput | null>(null)
  const [review, setReview] = useState<DiscoveryReview | null>(null)
  const [drafts, setDrafts] = useState<Record<number, DiscoveryEditorState>>({})
  const [completedBlockIndexes, setCompletedBlockIndexes] = useState<number[]>(
    []
  )
  const probeText = form.watch('text')
  const probeBaseUrl = form.watch('baseUrl')

  const groupsQuery = useQuery({ queryKey: ['groups'], queryFn: getGroups })
  const connection = connections.find(
    (item) => item.block_index === selectedBlockIndex
  )

  const createEditorState = (
    next: ChannelDiscoveryConnection
  ): DiscoveryEditorState => {
    const recommendedChoices = next.choices.filter(
      (choice) => choice.recommended
    )
    const recommendedModels = [
      ...new Set(recommendedChoices.map((choice) => choice.model)),
    ]
    return {
      formValues: {
        operation: 'create',
        channelId: next.matches[0]?.id ?? 0,
        baseUrl: next.base_url,
        name: next.suggested_name,
        tag: '',
        priority: 0,
        keyMode: 'append',
        syncConfiguration: true,
        enabled: true,
        reenable: false,
      },
      selectedModels: recommendedModels,
      mapping: Object.fromEntries(
        recommendedChoices
          .filter((choice) => choice.model !== choice.target)
          .map((choice) => [choice.model, choice.target])
      ),
      routes: {},
      acceptedKeyIndexes: [...(next.usable_key_indexes ?? [])],
      groups: ['default'],
      // Never probe an unselected model when discovery has no recommendation;
      // selecting a checkbox below chooses the first representative model.
      probeModel: selectChannelDiscoveryProbeModel(recommendedModels),
      lastProbeInput: null,
    }
  }

  const captureEditorState = (): DiscoveryEditorState => {
    const { text: _text, ...formValues } = form.getValues()
    return {
      formValues,
      selectedModels: [...selectedModels],
      mapping: { ...mapping },
      routes: { ...routes },
      acceptedKeyIndexes: [...acceptedKeyIndexes],
      groups: [...groups],
      probeModel,
      lastProbeInput,
    }
  }

  const restoreConnection = (
    next: ChannelDiscoveryConnection,
    state: DiscoveryEditorState
  ) => {
    form.reset({ text: form.getValues('text'), ...state.formValues })
    setSelectedBlockIndex(next.block_index)
    setSelectedModels([...state.selectedModels])
    setMapping({ ...state.mapping })
    setRoutes({ ...state.routes })
    setAcceptedKeyIndexes([...state.acceptedKeyIndexes])
    setGroups([...state.groups])
    setProbeModel(state.probeModel)
    setLastProbeInput(state.lastProbeInput)
    setReview(null)
  }

  const selectConnection = (next: ChannelDiscoveryConnection) => {
    const nextDrafts = { ...drafts }
    if (selectedBlockIndex !== null) {
      nextDrafts[selectedBlockIndex] = captureEditorState()
    }
    setDrafts(nextDrafts)
    restoreConnection(
      next,
      nextDrafts[next.block_index] ?? createEditorState(next)
    )
  }

  const resetDiscovery = () => {
    form.reset(discoveryDefaultValues)
    setConnections([])
    setSelectedBlockIndex(null)
    setSelectedModels([])
    setMapping({})
    setRoutes({})
    setAcceptedKeyIndexes([])
    setGroups(['default'])
    setProbeModel('')
    setLastProbeInput(null)
    setReview(null)
    setDrafts({})
    setCompletedBlockIndexes([])
  }

  const buildDraft = (values: DiscoveryFormValues): ChannelDiscoveryDraft => ({
    operation: values.operation,
    block_index: connection?.block_index ?? -1,
    channel_id: values.operation === 'update' ? values.channelId : undefined,
    base_url: values.baseUrl,
    accepted_key_indexes: acceptedKeyIndexes,
    selected_models: selectedModels,
    mapping,
    routes,
    name: values.operation === 'create' ? values.name : undefined,
    groups: values.operation === 'create' ? groups : undefined,
    tag: values.operation === 'create' ? values.tag : undefined,
    priority: values.operation === 'create' ? values.priority : undefined,
    enabled: values.enabled,
    key_mode: values.keyMode,
    sync_configuration: values.syncConfiguration,
    reenable: values.reenable,
  })

  const currentProbeInput = (): ChannelDiscoveryProbeInput => ({
    text: probeText,
    block_index: selectedBlockIndex ?? -1,
    key_index: acceptedKeyIndexes[0] ?? -1,
    base_url: probeBaseUrl,
    model: mapping[probeModel] ?? probeModel,
    models_path: connection?.models_path,
  })
  const hasProbed =
    lastProbeInput !== null &&
    isCurrentChannelDiscoveryProbe(lastProbeInput, currentProbeInput())

  const currentPreviewInput = (): ChannelDiscoveryPreviewInput => {
    const values = form.getValues()
    return { text: values.text, draft: buildDraft(values) }
  }

  const discoveryMutation = useMutation({
    mutationFn: discoverChannels,
    onSuccess: (data, requestText) => {
      if (!isCurrentChannelDiscoveryText(requestText, form.getValues('text'))) {
        return
      }
      setDrafts({})
      setCompletedBlockIndexes([])
      setConnections(data)
      const firstUsable = data.find((item) => item.usable)
      if (firstUsable) {
        restoreConnection(firstUsable, createEditorState(firstUsable))
      } else setSelectedBlockIndex(null)
    },
    onError: (error, requestText) => {
      if (isCurrentChannelDiscoveryText(requestText, form.getValues('text'))) {
        handleServerError(error)
      }
    },
  })

  const probeMutation = useMutation({
    mutationFn: probeChannelDiscovery,
    onSuccess: (result, input) => {
      if (!isCurrentChannelDiscoveryProbe(input, currentProbeInput())) return
      const nextRoutes = { ...result }
      if (
        nextRoutes.responses &&
        selectedModels.some(
          (model) => model.startsWith('gpt-5.5') || model.startsWith('gpt-5.6')
        )
      ) {
        nextRoutes.compact = nextRoutes.responses.replace(
          /\/responses$/,
          '/responses/compact'
        )
      }
      if (selectedModels.some(isChannelDiscoveryImageModel)) {
        nextRoutes.image_generations = '/v1/images/generations'
      }
      setRoutes(nextRoutes)
      setLastProbeInput(input)
    },
    onError: (error, input) => {
      if (isCurrentChannelDiscoveryProbe(input, currentProbeInput())) {
        handleServerError(error)
      }
    },
  })
  const failedProbeIsCurrent =
    probeMutation.isError &&
    probeMutation.variables !== undefined &&
    isCurrentChannelDiscoveryProbe(probeMutation.variables, currentProbeInput())
  let probeStatus: 'idle' | 'probing' | 'success' | 'error' = 'idle'
  let displayedProbeInput: ChannelDiscoveryProbeInput | null = null
  if (probeMutation.isPending) {
    probeStatus = 'probing'
    displayedProbeInput = probeMutation.variables
  } else if (failedProbeIsCurrent) {
    probeStatus = 'error'
    displayedProbeInput = probeMutation.variables
  } else if (hasProbed) {
    probeStatus = 'success'
    displayedProbeInput = lastProbeInput
  }
  const probeDisplayModel =
    displayedProbeInput?.model ?? mapping[probeModel] ?? probeModel
  const probeKeyIndex =
    displayedProbeInput?.key_index ?? acceptedKeyIndexes[0] ?? -1

  const previewMutation = useMutation({
    mutationFn: previewChannelDiscovery,
    onSuccess: (preview, input) => {
      if (!isCurrentChannelDiscoveryPreview(input, currentPreviewInput())) {
        return
      }
      setReview({ text: input.text, draft: input.draft, preview })
    },
    onError: (error, input) => {
      if (isCurrentChannelDiscoveryPreview(input, currentPreviewInput())) {
        handleServerError(error)
      }
    },
  })

  const applyMutation = useMutation({
    mutationFn: applyChannelDiscovery,
    onSuccess: async (_result, input) => {
      const completed = [
        ...new Set([...completedBlockIndexes, input.draft.block_index]),
      ]
      const nextDrafts = { ...drafts }
      delete nextDrafts[input.draft.block_index]
      setReview(null)
      await queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
      toast.success(t('Channel configuration applied'))
      const next = connections.find(
        (item) => item.usable && !completed.includes(item.block_index)
      )
      if (!next) {
        resetDiscovery()
        return
      }
      setCompletedBlockIndexes(completed)
      setDrafts(nextDrafts)
      restoreConnection(
        next,
        nextDrafts[next.block_index] ?? createEditorState(next)
      )
    },
    onError: handleServerError,
  })

  const setModelSelection = (choices: ChannelDiscoveryChoice[]) => {
    setLastProbeInput(null)
    const choicesByModel = new Map<string, ChannelDiscoveryChoice>()
    for (const choice of choices) {
      if (!choicesByModel.has(choice.model)) {
        choicesByModel.set(choice.model, choice)
      }
    }
    const values = [...choicesByModel.keys()].sort()
    setSelectedModels(values)
    if (!values.includes(probeModel)) {
      setProbeModel(selectChannelDiscoveryProbeModel(values))
    }
    setRoutes((currentRoutes) => {
      const updated = { ...currentRoutes }
      if (
        !values.some(
          (model) => model.startsWith('gpt-5.5') || model.startsWith('gpt-5.6')
        )
      ) {
        delete updated.compact
      }
      return updated
    })
    setMapping((current) => {
      const next: Record<string, string> = {}
      for (const choice of choicesByModel.values()) {
        if (current[choice.model] !== undefined) {
          next[choice.model] = current[choice.model]
        } else if (choice.model !== choice.target) {
          next[choice.model] = choice.target
        }
      }
      return next
    })
  }

  const toggleModel = (choice: ChannelDiscoveryChoice, checked: boolean) => {
    const choicesByModel = new Map(
      (connection?.choices ?? []).map((item) => [item.model, item])
    )
    const nextModels = new Set(selectedModels)
    if (checked) nextModels.add(choice.model)
    else nextModels.delete(choice.model)
    setModelSelection(
      [...nextModels].map(
        (model) =>
          choicesByModel.get(model) ?? {
            source: model,
            model,
            target: mapping[model] ?? model,
            recommended: false,
          }
      )
    )
  }

  const setRoute = (protocol: string, path: string | null) => {
    setRoutes((current) => {
      const next = { ...current }
      if (path === null) {
        delete next[protocol]
        if (protocol === 'responses') delete next.compact
      } else {
        next[protocol] = path
      }
      return next
    })
  }

  const handlePreview = form.handleSubmit((values) => {
    if (!connection || acceptedKeyIndexes.length === 0) {
      toast.error(t('Select at least one usable API key'))
      return
    }
    if (
      values.operation === 'create' &&
      (!values.name || groups.length === 0)
    ) {
      toast.error(t('Name and at least one group are required'))
      return
    }
    if (
      (values.operation === 'create' || values.syncConfiguration) &&
      (selectedModels.length === 0 ||
        !Object.keys(routes).some((protocol) => protocol !== 'models'))
    ) {
      toast.error(t('Select models and complete protocol probing first'))
      return
    }
    previewMutation.mutate({ text: values.text, draft: buildDraft(values) })
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Smart Recognition')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Breadcrumb>
        <Button
          variant='ghost'
          size='sm'
          className='text-muted-foreground hover:text-foreground h-auto gap-1 px-0 py-1 text-xs'
          render={<Link to='/channels' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('Back to Channels')}
        </Button>
      </SectionPageLayout.Breadcrumb>
      <SectionPageLayout.Content>
        <div
          className={`${channelSetupLayoutClasses.content} ${channelSetupLayoutClasses.mobileControls}`}
        >
          <div className={channelSetupLayoutClasses.workspace}>
            <ChannelDiscoverySourcePanel
              form={form}
              connections={connections.filter(
                (item) => !completedBlockIndexes.includes(item.block_index)
              )}
              totalCount={connections.length}
              completedCount={completedBlockIndexes.length}
              draftedBlockIndexes={[
                ...new Set([
                  ...Object.keys(drafts).map(Number),
                  ...(selectedBlockIndex === null ? [] : [selectedBlockIndex]),
                ]),
              ]}
              selectedBlockIndex={selectedBlockIndex}
              discovering={discoveryMutation.isPending}
              onTextChange={() => {
                setConnections([])
                setSelectedBlockIndex(null)
                setLastProbeInput(null)
                setReview(null)
              }}
              onReset={resetDiscovery}
              onDiscover={discoveryMutation.mutate}
              onSelect={selectConnection}
            />
            {!connection ? (
              <Empty className='min-h-64 border'>
                <EmptyHeader>
                  <EmptyMedia variant='icon'>
                    <HugeiconsIcon icon={AiScanIcon} strokeWidth={1.7} />
                  </EmptyMedia>
                  <EmptyTitle>{t('Configuration')}</EmptyTitle>
                  <EmptyDescription>
                    {t(
                      'Discover a usable connection to configure its models and endpoints.'
                    )}
                  </EmptyDescription>
                </EmptyHeader>
              </Empty>
            ) : (
              <ChannelDiscoveryWorkspace
                key={connection.block_index}
                form={form}
                connection={connection}
                groupOptions={groupsQuery.data?.data ?? ['default']}
                groups={groups}
                selectedModels={selectedModels}
                mapping={mapping}
                routes={routes}
                acceptedKeyIndexes={acceptedKeyIndexes}
                probeModel={probeModel}
                probing={probeMutation.isPending}
                probeStatus={probeStatus}
                probeDisplayModel={probeDisplayModel}
                probeKeyIndex={probeKeyIndex}
                onGroupsChange={setGroups}
                onAcceptedKeysChange={(indexes) => {
                  setAcceptedKeyIndexes(indexes)
                  setLastProbeInput(null)
                }}
                onProbeModelChange={(model) => {
                  setProbeModel(model)
                  setLastProbeInput(null)
                }}
                onProbe={() => probeMutation.mutate(currentProbeInput())}
                onToggleModel={toggleModel}
                onModelsChange={setModelSelection}
                onSetMapping={(source, target) => {
                  setLastProbeInput(null)
                  setSelectedModels((current) =>
                    [...new Set([...current, source])].sort()
                  )
                  setMapping((current) => ({ ...current, [source]: target }))
                }}
                onRemoveMapping={(source) => {
                  setLastProbeInput(null)
                  setMapping((current) => {
                    const next = { ...current }
                    delete next[source]
                    return next
                  })
                  if (!connection.models.includes(source)) {
                    setSelectedModels((current) =>
                      current.filter((model) => model !== source)
                    )
                  }
                }}
                onSetRoute={setRoute}
              />
            )}
          </div>
        </div>
        {connection ? (
          <PageFooterPortal>
            <div className={channelSetupLayoutClasses.footer}>
              {!canPreview ? (
                <p className='text-muted-foreground text-sm sm:mr-auto'>
                  {t(
                    'Channel write permission is required to generate a preview.'
                  )}
                </p>
              ) : null}
              <Button
                className={channelSetupLayoutClasses.mobileAction}
                onClick={handlePreview}
                disabled={!canPreview || previewMutation.isPending}
              >
                {previewMutation.isPending ? <Spinner /> : null}
                {previewMutation.isPending
                  ? t('Generating preview...')
                  : t('Review configuration')}
              </Button>
            </div>
          </PageFooterPortal>
        ) : null}
        <ChannelDiscoveryPreviewDialog
          open={review !== null}
          preview={review?.preview ?? null}
          applying={applyMutation.isPending}
          canApply={canApply}
          onOpenChange={(open) => {
            if (!open && !applyMutation.isPending) setReview(null)
          }}
          onConfirm={() => {
            if (!review) return
            applyMutation.mutate({
              text: review.text,
              draft: review.draft,
              preview_hash: review.preview.preview_hash,
              snapshot_hash: review.preview.snapshot_hash ?? '',
            })
          }}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
