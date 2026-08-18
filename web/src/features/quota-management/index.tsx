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
import { PlusSignIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { listCycles } from './api'
import { CreateCycleDialog } from './components/create-cycle-dialog'
import { CycleTable } from './components/cycle-table'
import { ManualQuotaDialog } from './components/manual-quota-dialog'
import { PageError, PageLoading } from './components/shared'
import { queryKeys } from './utils'

export function QuotaManagement() {
  const { t } = useTranslation()
  const companyQuotaModeEnabled = useSystemConfigStore(
    (state) => state.config.companyQuotaModeEnabled === true
  )
  const [cycleDialogOpen, setCycleDialogOpen] = useState(false)
  const [manualDialogOpen, setManualDialogOpen] = useState(false)
  const cyclesQuery = useQuery({
    queryKey: queryKeys.cycles,
    queryFn: listCycles,
  })
  let cyclesContent
  if (cyclesQuery.isPending) {
    cyclesContent = <PageLoading />
  } else if (cyclesQuery.isError) {
    cyclesContent = (
      <PageError
        title={t('Failed to load quota cycles')}
        error={cyclesQuery.error}
      />
    )
  } else {
    cyclesContent = <CycleTable cycles={cyclesQuery.data.cycles} />
  }
  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          <span className='inline-flex min-w-0 items-center gap-2'>
            <span className='truncate'>{t('Quota Management')}</span>
            <Badge variant='outline' className='shrink-0'>
              {t('Admin')}
            </Badge>
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            variant='outline'
            onClick={() => setManualDialogOpen(true)}
            disabled={!cyclesQuery.data || !companyQuotaModeEnabled}
          >
            {t('Manual adjustment')}
          </Button>
          <Button
            variant='outline'
            onClick={() => setCycleDialogOpen(true)}
            disabled={!cyclesQuery.data}
          >
            <HugeiconsIcon
              icon={PlusSignIcon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Create cycle')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>{cyclesContent}</SectionPageLayout.Content>
      </SectionPageLayout>
      {cyclesQuery.data && (
        <CreateCycleDialog
          open={cycleDialogOpen}
          onOpenChange={setCycleDialogOpen}
          defaults={cyclesQuery.data.defaults}
          recommendation={cyclesQuery.data.recommendation}
        />
      )}
      <ManualQuotaDialog
        open={manualDialogOpen}
        onOpenChange={setManualDialogOpen}
      />
    </>
  )
}
