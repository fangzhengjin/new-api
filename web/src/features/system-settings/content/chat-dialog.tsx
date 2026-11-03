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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChatPresetIcon } from '@/features/chat/components/chat-preset-icon'
import {
  isChatPresetIconName,
  resolveChatPresetIconName,
} from '@/features/chat/lib/chat-icons'
import {
  CHAT_SANDBOX_PERMISSIONS,
  getChatIframeSandbox,
  type ChatSandboxPermission,
} from '@/features/chat/lib/chat-links'

const createChatDialogSchema = (t: (key: string) => string) =>
  z.object({
    icon: z
      .string()
      .trim()
      .refine(
        (value) => value === '' || isChatPresetIconName(value),
        t('未找到该图标，请填写有效的 Lucide 图标名称')
      ),
    name: z.string().trim().min(1, t('Chat client name is required')),
    url: z.string().trim().min(1, t('URL is required')),
    openMode: z.enum(['embedded', 'new_tab']).optional(),
    sandbox: z.array(z.enum(CHAT_SANDBOX_PERMISSIONS)).optional(),
  })

type ChatDialogFormValues = z.infer<ReturnType<typeof createChatDialogSchema>>

const CHAT_DIALOG_FORM_ID = 'chat-dialog-form'

export type ChatEntryData = {
  icon?: string
  name: string
  url: string
  openMode?: 'embedded' | 'new_tab'
  sandbox?: ChatSandboxPermission[]
}

type ChatDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: ChatEntryData) => void
  editData?: ChatEntryData | null
}

export function ChatDialog({
  open,
  onOpenChange,
  onSave,
  editData,
}: ChatDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData
  const chatDialogSchema = createChatDialogSchema(t)

  const form = useForm<ChatDialogFormValues>({
    resolver: zodResolver(chatDialogSchema),
    defaultValues: {
      icon: '',
      name: '',
      url: '',
      openMode: 'embedded',
      sandbox: undefined,
    },
  })

  useEffect(() => {
    if (editData) {
      form.reset({ ...editData, icon: editData.icon ?? '' })
    } else {
      form.reset({
        icon: '',
        name: '',
        url: '',
        openMode: 'embedded',
        sandbox: undefined,
      })
    }
  }, [editData, form, open])

  const currentURL = form.watch('url')
  const icon = form.watch('icon') ?? ''
  const resolvedIconName = resolveChatPresetIconName(icon)
  const showIconPreview = icon.trim() === '' || resolvedIconName !== null
  const openMode = form.watch('openMode') ?? 'embedded'
  const sandbox = form.watch('sandbox')
  const isHttpURL = /^https?:\/\//i.test(currentURL)
  const isEmbedded = isHttpURL && openMode === 'embedded'
  const defaultSandbox = getChatIframeSandbox(
    currentURL,
    window.location.origin
  ).split(' ') as ChatSandboxPermission[]
  const effectiveSandbox = sandbox ?? defaultSandbox
  let isSameOrigin = false
  try {
    isSameOrigin =
      new URL(currentURL, window.location.origin).origin ===
      window.location.origin
  } catch {
    // URL validation reports the invalid value at submission.
  }
  const hasOriginIsolationRisk =
    isSameOrigin &&
    effectiveSandbox.includes('allow-scripts') &&
    effectiveSandbox.includes('allow-same-origin')
  const sandboxOptions: Array<{
    label: string
    value: ChatSandboxPermission
  }> = [
    { label: t('下载文件'), value: 'allow-downloads' },
    { label: t('提交表单'), value: 'allow-forms' },
    { label: t('显示模态对话框'), value: 'allow-modals' },
    { label: t('打开弹出窗口'), value: 'allow-popups' },
    {
      label: t('允许弹出窗口脱离沙箱'),
      value: 'allow-popups-to-escape-sandbox',
    },
    { label: t('使用演示模式'), value: 'allow-presentation' },
    { label: t('运行脚本'), value: 'allow-scripts' },
    { label: t('保留原始来源'), value: 'allow-same-origin' },
  ]

  const handleSubmit = (values: ChatDialogFormValues) => {
    onSave({
      ...(values.icon ? { icon: values.icon } : {}),
      name: values.name,
      url: values.url,
      openMode: isHttpURL ? (values.openMode ?? 'embedded') : undefined,
      ...(isEmbedded && values.sandbox !== undefined
        ? { sandbox: values.sandbox }
        : {}),
    })
    form.reset()
    onOpenChange(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEditMode ? t('Edit chat preset') : t('Add chat preset')}
      description={t('Configure a predefined chat link for end users.')}
      contentClassName='sm:max-w-[500px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={CHAT_DIALOG_FORM_ID}>
            {isEditMode ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={CHAT_DIALOG_FORM_ID}
          onSubmit={form.handleSubmit(handleSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='icon'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('菜单图标')}</FormLabel>
                <InputGroup>
                  <InputGroupAddon aria-hidden='true'>
                    {showIconPreview && (
                      <ChatPresetIcon name={resolvedIconName ?? undefined} />
                    )}
                  </InputGroupAddon>
                  <FormControl>
                    <InputGroupInput
                      placeholder={t('例如：Palette')}
                      autoComplete='off'
                      {...field}
                    />
                  </FormControl>
                </InputGroup>
                <FormDescription>
                  {t('可选，仅在该预设作为一级菜单项显示时使用')}{' '}
                  <a
                    href='https://lucide.dev/icons/'
                    target='_blank'
                    rel='noreferrer'
                    className='text-foreground underline underline-offset-4'
                  >
                    {t('查看 Lucide 图标')}
                  </a>
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Chat Client Name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Please enter chat client name')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Display name for this chat client.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='url'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('URL')}</FormLabel>
                <FormControl>
                  <Input placeholder={t('Please enter the URL')} {...field} />
                </FormControl>
                <FormDescription>
                  {t('The URL for this chat client.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {isHttpURL && (
            <FormField
              control={form.control}
              name='openMode'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('打开方式')}</FormLabel>
                  <Select
                    value={field.value ?? 'embedded'}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue>
                          {t(
                            field.value === 'new_tab' ? '新标签页' : '内嵌'
                          )}
                        </SelectValue>
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectItem value='embedded'>{t('内嵌')}</SelectItem>
                      <SelectItem value='new_tab'>{t('新标签页')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t('选择此 HTTP 链接在站内打开，还是在新标签页打开')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          {isEmbedded && (
            <>
              <FormField
                control={form.control}
                name='sandbox'
                render={({ field }) => {
                  const selected = field.value ?? defaultSandbox
                  let selectedLabel = t('已选择权限：{{count}} 项', {
                    count: selected.length,
                  })
                  if (field.value === undefined) {
                    selectedLabel = t('系统默认 · 已启用 {{count}} 项', {
                      count: selected.length,
                    })
                  } else if (selected.length === 0) {
                    selectedLabel = t('未选择任何权限')
                  }
                  return (
                    <FormItem>
                      <FormLabel>{t('嵌入页面权限')}</FormLabel>
                      <Select
                        items={sandboxOptions}
                        multiple
                        value={selected}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue>{selectedLabel}</SelectValue>
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {sandboxOptions.map((option) => (
                              <SelectItem
                                key={option.value}
                                value={option.value}
                              >
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <div className='flex flex-wrap items-start justify-between gap-2'>
                        <FormDescription className='min-w-0 flex-1'>
                          {t('选择嵌入页面可以使用的功能。不作修改时沿用系统默认权限')}
                        </FormDescription>
                        {field.value !== undefined && (
                          <Button
                            type='button'
                            variant='link'
                            size='sm'
                            className='h-auto px-0 py-0'
                            onClick={() => field.onChange(undefined)}
                          >
                            {t('使用系统默认权限')}
                          </Button>
                        )}
                      </div>
                      <FormMessage />
                    </FormItem>
                  )
                }}
              />
              {hasOriginIsolationRisk && (
                <Alert variant='destructive'>
                  <AlertTitle>
                    {t('高风险权限组合')}
                  </AlertTitle>
                  <AlertDescription>
                    {t('同时允许脚本运行和保留原始来源后，iframe 将不再具备可靠的隔离能力')}
                  </AlertDescription>
                </Alert>
              )}
            </>
          )}
        </form>
      </Form>
    </Dialog>
  )
}
