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
import { useQuery } from '@tanstack/react-query'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { getServerErrorMessage } from '@/lib/handle-server-error'

import { getSelfTemporaryQuota, temporaryQuotaQueryKeys } from './api'
import { TemporaryQuotaDetailSheet } from './components/temporary-quota-detail-sheet'
import { TemporaryQuotaDialog } from './components/temporary-quota-dialog'
import { TemporaryQuotaPrimaryAction } from './components/temporary-quota-primary-action'
import { TemporaryQuotaTable } from './components/temporary-quota-table'
import type { SelfTemporaryQuotaRequest } from './types'

export function TemporaryQuota() {
  const { t } = useTranslation()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [selectedRequest, setSelectedRequest] =
    useState<SelfTemporaryQuotaRequest | null>(null)
  const overview = useQuery({
    queryKey: temporaryQuotaQueryKeys.overview,
    queryFn: getSelfTemporaryQuota,
  })
  const viewRequest = useCallback(
    (request: SelfTemporaryQuotaRequest) => setSelectedRequest(request),
    []
  )

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Temporary quota')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <TemporaryQuotaPrimaryAction
            overview={overview.data}
            onOpen={() => setDialogOpen(true)}
          />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          {overview.isError && (
            <Alert variant='destructive' className='mb-3'>
              <AlertTitle>{t('Temporary quota')}</AlertTitle>
              <AlertDescription>
                {getServerErrorMessage(overview.error)}
              </AlertDescription>
            </Alert>
          )}
          <TemporaryQuotaTable onView={viewRequest} />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <TemporaryQuotaDialog
        open={dialogOpen}
        overview={overview.data}
        onOpenChange={setDialogOpen}
      />
      <TemporaryQuotaDetailSheet
        request={selectedRequest}
        onOpenChange={(open) => {
          if (!open) setSelectedRequest(null)
        }}
      />
    </>
  )
}
