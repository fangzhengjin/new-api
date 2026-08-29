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
import { Plus, Search } from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  parseChatConfigEntries,
  serializeChatConfigEntry,
  type ChatConfigEntry,
} from '@/features/chat/lib/chat-links'

import { ChatDialog, type ChatEntryData } from './chat-dialog'

type ChatSettingsVisualEditorProps = {
  value: string
  onChange: (value: string) => void
}

type ChatEntry = ChatConfigEntry

export function ChatSettingsVisualEditor({
  value,
  onChange,
}: ChatSettingsVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<ChatEntry | null>(null)

  const chats = useMemo(() => parseChatConfigEntries(value), [value])

  const filteredChats = useMemo(() => {
    if (!searchText) return chats
    const lowerSearch = searchText.toLowerCase()
    return chats.filter(
      (chat) =>
        chat.name.toLowerCase().includes(lowerSearch) ||
        chat.url.toLowerCase().includes(lowerSearch)
    )
  }, [chats, searchText])

  const handleSave = (data: ChatEntryData) => {
    const entry = {
      ...data,
      enabled: editData?.enabled ?? true,
    }
    const updated = editData
      ? chats.map((chat) => (chat.name === editData.name ? entry : chat))
      : [...chats, entry]
    onChange(JSON.stringify(updated.map(serializeChatConfigEntry), null, 2))
  }

  const handleDelete = (name: string) => {
    onChange(
      JSON.stringify(
        chats
          .filter((chat) => chat.name !== name)
          .map(serializeChatConfigEntry),
        null,
        2
      )
    )
  }

  const handleEnabledChange = (chat: ChatEntry, enabled: boolean) => {
    onChange(
      JSON.stringify(
        chats.map((item) =>
          serializeChatConfigEntry(
            item.name === chat.name ? { ...item, enabled } : item
          )
        ),
        null,
        2
      )
    )
  }

  const handleEdit = (chat: ChatEntry) => {
    setEditData(chat)
    setDialogOpen(true)
  }

  const handleAdd = () => {
    setEditData(null)
    setDialogOpen(true)
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-4'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search chat presets...')}
            aria-label={t('Search chat presets')}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className='pl-9'
          />
        </div>
        <Button type='button' onClick={handleAdd}>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add chat preset')}
        </Button>
      </div>

      <StaticDataTable
        data={filteredChats}
        getRowKey={(chat) => chat.name}
        emptyContent={
          searchText
            ? t('No chat presets match your search')
            : t(
                'No chat presets configured. Click "Add chat preset" to get started.'
              )
        }
        columns={[
          {
            id: 'name',
            header: t('Chat Client Name'),
            cellClassName: 'font-medium',
            cell: (chat) => chat.name,
          },
          {
            id: 'url',
            header: t('URL'),
            cellClassName: 'max-w-md truncate font-mono text-sm',
            cell: (chat) => chat.url,
          },
          {
            id: 'open-mode',
            header: t('Open mode'),
            cell: (chat) =>
              chat.openMode
                ? t(chat.openMode === 'new_tab' ? 'New tab' : 'Embedded')
                : '—',
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (chat) => (
              <div className='flex items-center gap-2'>
                <Switch
                  checked={chat.enabled}
                  onCheckedChange={(enabled) =>
                    handleEnabledChange(chat, enabled)
                  }
                  aria-label={t('Enable {{parameter}}', {
                    parameter: chat.name,
                  })}
                />
                <span className='text-muted-foreground text-sm'>
                  {t(chat.enabled ? 'Enabled' : 'Disabled')}
                </span>
              </div>
            ),
          },
          {
            id: 'actions',
            header: t('Actions'),
            className: 'text-right',
            cellClassName: 'text-right',
            cell: (chat) => (
              <StaticRowActions
                editLabel={t('Edit')}
                deleteLabel={t('Delete')}
                menuLabel={t('Open menu')}
                onEdit={() => handleEdit(chat)}
                onDelete={() => handleDelete(chat.name)}
              />
            ),
          },
        ]}
      />

      <ChatDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
      />
    </div>
  )
}
