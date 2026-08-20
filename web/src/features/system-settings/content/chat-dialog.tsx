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
        t('Icon not found. Enter a valid Lucide icon name.')
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
    { label: t('Download files'), value: 'allow-downloads' },
    { label: t('Submit forms'), value: 'allow-forms' },
    { label: t('Show modal dialogs'), value: 'allow-modals' },
    { label: t('Open pop-up windows'), value: 'allow-popups' },
    {
      label: t('Let pop-ups leave the sandbox'),
      value: 'allow-popups-to-escape-sandbox',
    },
    { label: t('Use presentation mode'), value: 'allow-presentation' },
    { label: t('Run scripts'), value: 'allow-scripts' },
    { label: t('Keep the original origin'), value: 'allow-same-origin' },
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
                <FormLabel>{t('Menu icon')}</FormLabel>
                <InputGroup>
                  <InputGroupAddon aria-hidden='true'>
                    {showIconPreview && (
                      <ChatPresetIcon name={resolvedIconName ?? undefined} />
                    )}
                  </InputGroupAddon>
                  <FormControl>
                    <InputGroupInput
                      placeholder={t('e.g. Palette')}
                      autoComplete='off'
                      {...field}
                    />
                  </FormControl>
                </InputGroup>
                <FormDescription>
                  {t(
                    'Optional. Used only when this preset appears as a top-level menu item.'
                  )}{' '}
                  <a
                    href='https://lucide.dev/icons/'
                    target='_blank'
                    rel='noreferrer'
                    className='text-foreground underline underline-offset-4'
                  >
                    {t('Browse Lucide icons')}
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
                  <FormLabel>{t('Open mode')}</FormLabel>
                  <Select
                    value={field.value ?? 'embedded'}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue>
                          {t(
                            field.value === 'new_tab' ? 'New tab' : 'Embedded'
                          )}
                        </SelectValue>
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectItem value='embedded'>{t('Embedded')}</SelectItem>
                      <SelectItem value='new_tab'>{t('New tab')}</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t(
                      'Choose whether this HTTP link opens inside the site or in a new tab.'
                    )}
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
                  let selectedLabel = t('Selected permissions: {{count}}', {
                    count: selected.length,
                  })
                  if (field.value === undefined) {
                    selectedLabel = t('System default · {{count}} enabled', {
                      count: selected.length,
                    })
                  } else if (selected.length === 0) {
                    selectedLabel = t('No permissions selected')
                  }
                  return (
                    <FormItem>
                      <FormLabel>{t('Embedded page permissions')}</FormLabel>
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
                          {t(
                            'Choose what the embedded page may use. Leave this unchanged to follow the system defaults.'
                          )}
                        </FormDescription>
                        {field.value !== undefined && (
                          <Button
                            type='button'
                            variant='link'
                            size='sm'
                            className='h-auto px-0 py-0'
                            onClick={() => field.onChange(undefined)}
                          >
                            {t('Use system defaults')}
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
                    {t('High-risk permission combination')}
                  </AlertTitle>
                  <AlertDescription>
                    {t(
                      'Scripts and original-origin access together remove reliable iframe isolation.'
                    )}
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
