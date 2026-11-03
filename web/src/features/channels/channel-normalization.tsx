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
import {
  AiScanIcon,
  ArrowLeft01Icon,
  CheckmarkCircle02Icon,
  ReloadIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { PageFooterPortal, SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Progress } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  applyChannelNormalization,
  getCurrentChannelNormalization,
  startChannelNormalization,
  type ChannelNormalizationData,
  type ChannelNormalizationSelection,
} from './channel-setup'
import { ChannelNormalizationPreview } from './components/channel-normalization-preview'
import { channelsQueryKeys } from './lib'
import { channelSetupLayoutClasses } from './lib/channel-discovery-form'
import {
  countNormalizationChanges,
  createDefaultNormalizationSelection,
  hasNormalizationChanges,
  isNormalizationSelectionValid,
} from './lib/channel-normalization-selection'

const normalizationQueryKey = ['channels', 'normalization', 'current'] as const

/**
 * Coordinates the resumable scan, editable preview, and atomic apply flow.
 * @returns The complete batch-normalization page.
 */
export function ChannelNormalization() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const canScan = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.OPERATE
  )
  const canApply = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const [includeDisabled, setIncludeDisabled] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [selections, setSelections] = useState<
    Record<number, ChannelNormalizationSelection>
  >({})
  const initializedTaskId = useRef<string | null>(null)

  const taskQuery = useQuery({
    queryKey: normalizationQueryKey,
    queryFn: getCurrentChannelNormalization,
    retry: false,
    refetchInterval: (query) => {
      const status = query.state.data?.task.status
      return status === 'pending' || status === 'running' ? 1500 : false
    },
  })
  const data = taskQuery.data
  const task = data?.task
  const taskId = task?.task_id
  const taskStatus = task?.status
  const taskAppliedAt = task?.result?.applied_at
  const isActive = task?.status === 'pending' || task?.status === 'running'
  const result = task?.result
  const hasPreviewItems = (result?.items?.length ?? 0) > 0
  const selectedChanges = Object.values(selections).filter(
    hasNormalizationChanges
  )
  const selectedActionCount = selectedChanges.reduce(
    (count, selection) => count + countNormalizationChanges(selection),
    0
  )
  const hasInvalidSelection = selectedChanges.some(
    (selection) => !isNormalizationSelectionValid(selection)
  )

  const resetSafeDefaults = useCallback(
    (nextData: ChannelNormalizationData | null) => {
      const nextSelections: Record<number, ChannelNormalizationSelection> = {}
      for (const item of nextData?.task.result?.items ?? []) {
        const metadata = nextData?.channels[String(item.channel_id)] ?? {
          models: [],
          model_mapping: {},
        }
        nextSelections[item.channel_id] = createDefaultNormalizationSelection(
          item,
          metadata
        )
      }
      setSelections(nextSelections)
    },
    []
  )

  useEffect(() => {
    if (
      !taskId ||
      taskStatus !== 'succeeded' ||
      taskAppliedAt ||
      initializedTaskId.current === taskId
    ) {
      return
    }
    // Refetches may refresh channel metadata for the same completed task; they
    // must not discard model and mapping choices the administrator already made.
    initializedTaskId.current = taskId
    resetSafeDefaults(data ?? null)
  }, [data, resetSafeDefaults, taskAppliedAt, taskId, taskStatus])

  const startMutation = useMutation({
    mutationFn: startChannelNormalization,
    onSuccess: (nextData) => {
      queryClient.setQueryData(normalizationQueryKey, nextData)
      setSelections({})
    },
    onError: (error: unknown) => handleServerError(error),
  })

  const applyMutation = useMutation({
    mutationFn: applyChannelNormalization,
    onSuccess: async (applied) => {
      setConfirmOpen(false)
      await Promise.all([
        taskQuery.refetch(),
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all }),
      ])
      toast.success(
        t('已规范化 {{count}} 个渠道', { count: applied.updated })
      )
    },
    onError: (error: unknown) => handleServerError(error),
  })

  const startScan = () => startMutation.mutate(includeDisabled)
  const progressTotal = task?.state?.total ?? 0
  const progressProcessed = task?.state?.processed ?? 0
  const progressValue =
    progressTotal > 0
      ? Math.round((progressProcessed / progressTotal) * 100)
      : 0

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('批量规范化')}</SectionPageLayout.Title>
      <SectionPageLayout.Breadcrumb>
        <Button
          variant='ghost'
          size='sm'
          className='text-muted-foreground hover:text-foreground h-auto gap-1 px-0 py-1 text-xs'
          render={<Link to='/channels' />}
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
          {t('返回渠道')}
        </Button>
      </SectionPageLayout.Breadcrumb>
      <SectionPageLayout.Content>
        <div
          className={`${channelSetupLayoutClasses.content} ${channelSetupLayoutClasses.mobileControls}`}
        >
          {(() => {
            if (taskQuery.isLoading) {
              return (
                <div
                  className='grid min-h-56 place-items-center'
                  aria-busy='true'
                >
                  <Spinner
                    className='size-6'
                    aria-label={t('正在加载规范化任务')}
                  />
                </div>
              )
            }
            if (taskQuery.isError) {
              return (
                <Alert variant='destructive'>
                  <AlertTitle>
                    {t('无法加载规范化状态')}
                  </AlertTitle>
                  <AlertDescription className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
                    <span>{t('Request failed')}</span>
                    <Button
                      variant='outline'
                      onClick={() => void taskQuery.refetch()}
                    >
                      {t('Retry')}
                    </Button>
                  </AlertDescription>
                </Alert>
              )
            }
            if (isActive) {
              return (
                <Card>
                  <CardHeader>
                    <CardTitle>
                      {t('正在扫描渠道配置')}
                    </CardTitle>
                    <CardDescription className='flex flex-col gap-1'>
                      <span>
                        {task?.status === 'pending'
                          ? t('正在等待任务执行器……')
                          : t('正在检查上游模型和规范化规则……')}
                      </span>
                      <span>
                        {t('扫描在后台运行。离开此页面不会停止任务，完成后也绝不会自动应用修改。')}
                      </span>
                    </CardDescription>
                  </CardHeader>
                  <CardContent className='space-y-3'>
                    <Progress
                      value={progressValue}
                      aria-label={t('扫描进度')}
                    />
                    <div className='text-muted-foreground flex flex-wrap justify-between gap-2 text-sm tabular-nums'>
                      <span>
                        {t('已处理 {{processed}} / {{total}} 个渠道', {
                          processed: progressProcessed,
                          total: progressTotal,
                        })}
                      </span>
                      <span>{progressValue}%</span>
                    </div>
                    {task?.state?.current_channel_id ? (
                      <p className='text-muted-foreground text-sm'>
                        {t('当前渠道：#{{id}}', {
                          id: task.state.current_channel_id,
                        })}
                      </p>
                    ) : null}
                  </CardContent>
                </Card>
              )
            }
            if (task?.status === 'succeeded' && !result?.applied_at) {
              return (
                <>
                  <ChannelNormalizationPreview
                    data={data as ChannelNormalizationData}
                    selections={selections}
                    onSelectionChange={(channelId, selection) =>
                      setSelections((current) => ({
                        ...current,
                        [channelId]: selection,
                      }))
                    }
                    onResetSafeDefaults={() => resetSafeDefaults(data ?? null)}
                  />
                  <PageFooterPortal>
                    {hasPreviewItems ? (
                      <div className={channelSetupLayoutClasses.footer}>
                        <p className='text-muted-foreground text-sm sm:mr-auto'>
                          {t('已选变更：{{changes}} · 渠道：{{channels}}',
                            {
                              changes: selectedActionCount,
                              channels: selectedChanges.length,
                            }
                          )}
                        </p>
                        <div className='flex flex-col-reverse gap-2 sm:flex-row'>
                          <Button
                            variant='outline'
                            className={channelSetupLayoutClasses.mobileAction}
                            disabled={!canScan || startMutation.isPending}
                            onClick={startScan}
                          >
                            <HugeiconsIcon icon={ReloadIcon} strokeWidth={2} />
                            {t('重新扫描')}
                          </Button>
                          <Button
                            className={channelSetupLayoutClasses.mobileAction}
                            disabled={!canApply || selectedChanges.length === 0}
                            onClick={() => {
                              if (hasInvalidSelection) {
                                toast.error(
                                  t('映射目标不能为空')
                                )
                                return
                              }
                              setConfirmOpen(true)
                            }}
                          >
                            {t('Apply selected changes')}
                          </Button>
                        </div>
                      </div>
                    ) : (
                      <div className='flex justify-end'>
                        <Button
                          variant='outline'
                          className='max-sm:min-h-11 max-sm:w-full'
                          disabled={!canScan || startMutation.isPending}
                          onClick={startScan}
                        >
                          <HugeiconsIcon icon={ReloadIcon} strokeWidth={2} />
                          {t('重新扫描')}
                        </Button>
                      </div>
                    )}
                  </PageFooterPortal>
                </>
              )
            }
            if (task?.status === 'failed') {
              return (
                <Alert variant='destructive'>
                  <AlertTitle>{t('规范化扫描失败')}</AlertTitle>
                  <AlertDescription className='space-y-3'>
                    <p>
                      {task.error ||
                        t('任务失败，但未返回错误信息。')}
                    </p>
                    <Button
                      variant='outline'
                      disabled={!canScan || startMutation.isPending}
                      onClick={startScan}
                    >
                      {t('重试')}
                    </Button>
                  </AlertDescription>
                </Alert>
              )
            }
            return (
              <Card>
                <CardHeader>
                  <CardTitle className='flex items-center gap-2'>
                    <HugeiconsIcon
                      icon={
                        result?.applied_at ? CheckmarkCircle02Icon : AiScanIcon
                      }
                      strokeWidth={1.8}
                    />
                    {result?.applied_at
                      ? t('上次规范化已应用')
                      : t('开始规范化扫描')}
                  </CardTitle>
                  <CardDescription className='flex flex-col gap-1'>
                    <span>
                      {result?.applied_at
                        ? t('运行新扫描以检查后续上游变化。')
                        : t('扫描会生成可编辑的模型和映射修改预览。')}
                    </span>
                    <span>
                      {t('扫描在后台运行。离开此页面不会停止任务，完成后也绝不会自动应用修改。')}
                    </span>
                  </CardDescription>
                </CardHeader>
                <CardContent className='space-y-4'>
                  <Field orientation='horizontal'>
                    <div>
                      <FieldLabel htmlFor='normalization-disabled'>
                        {t('获取已禁用渠道的上游模型')}
                      </FieldLabel>
                      <FieldDescription>
                        {t('关闭时，不会请求已禁用渠道的上游，仍会根据已存配置进行规范化。')}
                      </FieldDescription>
                    </div>
                    <Switch
                      id='normalization-disabled'
                      checked={includeDisabled}
                      onCheckedChange={setIncludeDisabled}
                    />
                  </Field>
                  <Button
                    onClick={startScan}
                    disabled={!canScan || startMutation.isPending}
                    className='max-sm:min-h-11 max-sm:w-full'
                  >
                    {startMutation.isPending ? (
                      <Spinner />
                    ) : (
                      <HugeiconsIcon icon={AiScanIcon} strokeWidth={2} />
                    )}
                    {startMutation.isPending
                      ? t('正在启动……')
                      : t('开始扫描')}
                  </Button>
                </CardContent>
              </Card>
            )
          })()}
        </div>

        <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
          <DialogContent className={channelSetupLayoutClasses.mobileControls}>
            <DialogHeader>
              <DialogTitle>{t('应用规范化修改？')}</DialogTitle>
              <DialogDescription>
                {t('所有选中渠道将以原子方式更新。如果扫描后任一渠道发生变化，则不会应用任何修改，必须重新扫描。')}
              </DialogDescription>
            </DialogHeader>
            <p className='text-sm font-medium'>
              {t('已选变更：{{changes}} · 渠道：{{channels}}', {
                changes: selectedActionCount,
                channels: selectedChanges.length,
              })}
            </p>
            <DialogFooter>
              <Button
                variant='outline'
                onClick={() => setConfirmOpen(false)}
                disabled={applyMutation.isPending}
              >
                {t('Cancel')}
              </Button>
              <Button
                disabled={
                  applyMutation.isPending ||
                  selectedChanges.length === 0 ||
                  hasInvalidSelection
                }
                onClick={() => {
                  if (!task) return
                  applyMutation.mutate({
                    task_id: task.task_id,
                    channels: selectedChanges,
                  })
                }}
              >
                {applyMutation.isPending
                  ? t('Applying...')
                  : t('Apply changes')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
