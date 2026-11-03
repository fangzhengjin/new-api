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
import { useEffect, useId, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { JsonCodeEditor } from '@/components/json-code-editor'
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
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { parseChatConfigEntries } from '@/features/chat/lib/chat-links'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { ChatSettingsVisualEditor } from './chat-settings-visual-editor'
import { formatJsonForEditor, normalizeJsonString } from './utils'

const createChatSchema = (t: (key: string) => string) =>
  z.object({
    ChatMenuCollapseThreshold: z
      .number()
      .int(t('请输入 0～20 的整数'))
      .min(0, t('请输入 0～20 的整数'))
      .max(20, t('请输入 0～20 的整数')),
    Chats: z.string().superRefine((value, ctx) => {
      try {
        const parsed = JSON.parse(value || '[]')
        if (!Array.isArray(parsed)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('Expected a JSON array.'),
          })
          return
        }
        if (parseChatConfigEntries(parsed).length !== parsed.length) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('每个预设必须包含唯一名称、URL、启用状态，可选的有效打开方式，以及可选的有效沙箱权限'),
          })
        }
      } catch {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: t('Invalid JSON string.'),
        })
      }
    }),
  })

type ChatSettingsFormValues = z.infer<ReturnType<typeof createChatSchema>>

type ChatSettingsSectionProps = {
  defaultValue: string
  defaultThreshold: number
  builtInValue: string
  builtInThreshold: number
}

export function ChatSettingsSection(props: ChatSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [editMode, setEditMode] = useState<'visual' | 'json'>('visual')
  const thresholdInputId = useId()

  const chatSchema = createChatSchema(t)
  const formatted = formatJsonForEditor(props.defaultValue, '[]')
  const builtInDefaultsResult = chatSchema.safeParse({
    Chats: formatJsonForEditor(props.builtInValue, '[]'),
    ChatMenuCollapseThreshold: props.builtInThreshold,
  })
  const builtInDefaults = builtInDefaultsResult.success
    ? builtInDefaultsResult.data
    : null
  const form = useForm<ChatSettingsFormValues>({
    resolver: zodResolver(chatSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues: {
      Chats: formatted,
      ChatMenuCollapseThreshold: props.defaultThreshold,
    },
  })

  const initialNormalizedRef = useRef(
    normalizeJsonString(props.defaultValue, '[]')
  )
  const initialThresholdRef = useRef(props.defaultThreshold)

  useEffect(() => {
    form.reset({
      Chats: formatJsonForEditor(props.defaultValue, '[]'),
      ChatMenuCollapseThreshold: props.defaultThreshold,
    })
    initialNormalizedRef.current = normalizeJsonString(props.defaultValue, '[]')
    initialThresholdRef.current = props.defaultThreshold
  }, [form, props.defaultThreshold, props.defaultValue])

  const onSubmit = async (values: ChatSettingsFormValues) => {
    const normalized = normalizeJsonString(values.Chats, '[]')
    const updates = []
    if (values.ChatMenuCollapseThreshold !== initialThresholdRef.current) {
      updates.push({
        key: 'ChatMenuCollapseThreshold',
        value: values.ChatMenuCollapseThreshold,
      })
    }
    if (normalized !== initialNormalizedRef.current) {
      updates.push({ key: 'Chats', value: normalized })
    }
    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const variables = [
    {
      name: '{address}',
      description: t('当前 API 地址，已进行 URL 编码'),
    },
    {
      name: '{theme}',
      description: t('当前主题：light 或 dark'),
    },
    {
      name: '{key}',
      description: t('所选 Token，用于直接导入客户端的链接'),
    },
    {
      name: '{cherryConfig}',
      description: t('Cherry Studio 导入配置'),
    },
    {
      name: '{aionuiConfig}',
      description: t('AionUI 导入配置'),
    },
    {
      name: '{deepchatConfig}',
      description: t('DeepChat 导入配置'),
    },
    {
      name: '{authCode}',
      description: t('一次性授权码，一分钟内有效且只能使用一次'),
    },
    {
      name: '{textModels}',
      description: t('所选 Token 可用的文本模型，以逗号分隔'),
    },
    {
      name: '{imageModels}',
      description: t('所选 Token 可用的图像模型，以逗号分隔'),
    },
    {
      name: '{videoModels}',
      description: t('所选 Token 可用的视频模型，以逗号分隔'),
    },
  ]
  const currentValues = form.watch()
  const usesBuiltInDefaults =
    builtInDefaults !== null &&
    currentValues.ChatMenuCollapseThreshold ===
      builtInDefaults.ChatMenuCollapseThreshold &&
    normalizeJsonString(currentValues.Chats, '[]') ===
      normalizeJsonString(builtInDefaults.Chats, '[]')

  return (
    <SettingsSection title={t('Chat Presets')}>
      <Form {...form}>
        {/* eslint-disable-next-line react-hooks/refs */}
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={() => builtInDefaults && form.reset(builtInDefaults)}
            isSaving={updateOption.isPending}
            isResetDisabled={!builtInDefaults || usesBuiltInDefaults}
            saveLabel='Save chat settings'
            resetLabel='Restore defaults'
          />
          <FormField
            control={form.control}
            name='ChatMenuCollapseThreshold'
            render={({ field }) => (
              <FormItem data-settings-form-span='full'>
                <FormLabel htmlFor={thresholdInputId} className='sr-only'>
                  {t('聊天菜单收纳阈值')}
                </FormLabel>
                <div className='flex flex-wrap items-center gap-2 text-sm'>
                  <span>{t('入口超过')}</span>
                  <FormControl>
                    <Input
                      {...field}
                      id={thresholdInputId}
                      type='number'
                      min={0}
                      max={20}
                      inputMode='numeric'
                      className='h-8 w-16'
                      value={Number.isNaN(field.value) ? '' : field.value}
                      onChange={(event) =>
                        field.onChange(
                          event.target.value === ''
                            ? Number.NaN
                            : Number(event.target.value)
                        )
                      }
                      aria-invalid={Boolean(
                        form.formState.errors.ChatMenuCollapseThreshold
                      )}
                    />
                  </FormControl>
                  <span>{t('个时，收进“聊天”菜单')}</span>
                </div>
                <FormMessage />
              </FormItem>
            )}
          />
          <Separator />
          <Tabs
            value={editMode}
            onValueChange={(value) => setEditMode(value as 'visual' | 'json')}
          >
            <TabsList className='grid w-full grid-cols-2'>
              <TabsTrigger value='visual'>{t('Visual')}</TabsTrigger>
              <TabsTrigger value='json'>{t('JSON')}</TabsTrigger>
            </TabsList>

            <TabsContent value='visual' className='mt-6'>
              <FormField
                control={form.control}
                name='Chats'
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <ChatSettingsVisualEditor
                        value={field.value}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='json' className='mt-6'>
              <FormField
                control={form.control}
                name='Chats'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Chat configuration JSON')}</FormLabel>
                    <FormControl>
                      <JsonCodeEditor
                        value={field.value}
                        onChange={field.onChange}
                        name={field.name}
                        onBlur={field.onBlur}
                        textareaRef={field.ref}
                        placeholder={t('[{"name":"Example","url":"https://chat.example","enabled":true,"icon":"Palette","open_mode":"embedded"}]')}
                        heightClassName='h-72 min-h-72 max-h-72'
                        aria-invalid={Boolean(form.formState.errors.Chats)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('每项必须包含 name、url 和 enabled，icon、open_mode 和 sandbox 为可选字段')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>
          </Tabs>
          <Separator />
          <div className='space-y-3' data-settings-form-span='full'>
            <div className='space-y-1'>
              <h3 className='text-sm font-medium'>{t('内置变量')}</h3>
              <p className='text-muted-foreground text-sm'>
                {t('只有打开预设时才会替换变量，已保存的设置不会显示真实 Token 值')}
              </p>
            </div>
            <div className='overflow-x-auto rounded-lg border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('变量')}</TableHead>
                    <TableHead>{t('说明')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {variables.map((variable) => (
                    <TableRow key={variable.name}>
                      <TableCell>
                        <code className='text-xs'>{variable.name}</code>
                      </TableCell>
                      <TableCell className='min-w-72'>
                        {variable.description}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
