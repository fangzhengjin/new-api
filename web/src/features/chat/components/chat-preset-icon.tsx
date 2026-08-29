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
import { MessageSquare, type LucideProps } from 'lucide-react'
import { DynamicIcon } from 'lucide-react/dynamic'

import { resolveChatPresetIconName } from '../lib/chat-icons'

type ChatPresetIconProps = LucideProps & {
  name?: string
  fallback?: React.ElementType
}

/** Renders a configured Lucide icon with the existing menu icon as fallback. */
export function ChatPresetIcon(props: ChatPresetIconProps) {
  const { name, fallback: Fallback = MessageSquare, ...iconProps } = props
  const resolvedName = resolveChatPresetIconName(name)

  if (!resolvedName) {
    return <Fallback {...iconProps} data-chat-preset-icon='message-square' />
  }

  return (
    <DynamicIcon
      {...iconProps}
      name={resolvedName}
      data-chat-preset-icon={resolvedName}
      fallback={() => <Fallback {...iconProps} />}
    />
  )
}
