import type { ParseKeys } from 'i18next'
import type { ComponentType } from 'react'
import { AccountIcon, AccountsIcon, ChartIcon, CheckIcon, HeirloomIcon, HomeIcon, LinkIcon, MapIcon, MembersIcon, PeopleIcon, PhotoIcon, ResearchIcon, ShareIcon, SourceIcon, TranscribeIcon, TransferIcon, TreeIcon, WallChartIcon } from './icons'

export interface NavItem {
  to: string
  key: ParseKeys
  Icon: ComponentType
  end?: boolean
  /** One of the links on the phone bar; the rest are in its More menu. */
  phone?: boolean
  /** Shown to administrators only. */
  adminOnly?: boolean
}

export interface NavGroup {
  id: string
  key: ParseKeys
  items: NavItem[]
}

export const homeItem: NavItem = { to: '/', key: 'nav.dashboard', Icon: HomeIcon, end: true, phone: true }

export const navGroups: NavGroup[] = [
  {
    id: 'tree',
    key: 'nav.groups.tree',
    items: [
      { to: '/people', key: 'nav.people', Icon: PeopleIcon, phone: true },
      { to: '/tree', key: 'nav.tree', Icon: TreeIcon, phone: true },
      { to: '/chart', key: 'nav.chart', Icon: WallChartIcon },
      { to: '/relationship', key: 'nav.relationship', Icon: LinkIcon },
    ],
  },
  {
    id: 'records',
    key: 'nav.groups.records',
    items: [
      { to: '/sources', key: 'nav.sources', Icon: SourceIcon, phone: true },
      { to: '/media', key: 'nav.media', Icon: PhotoIcon },
      { to: '/transcribe', key: 'nav.transcribe', Icon: TranscribeIcon },
      { to: '/heirlooms', key: 'nav.heirlooms', Icon: HeirloomIcon },
    ],
  },
  {
    id: 'explore',
    key: 'nav.groups.explore',
    items: [
      { to: '/map', key: 'nav.map', Icon: MapIcon },
      { to: '/stats', key: 'nav.stats', Icon: ChartIcon },
    ],
  },
  {
    id: 'work',
    key: 'nav.groups.work',
    items: [
      { to: '/research', key: 'nav.research', Icon: ResearchIcon },
      { to: '/quality', key: 'nav.quality', Icon: CheckIcon },
    ],
  },
  {
    id: 'settings',
    key: 'nav.groups.settings',
    items: [
      { to: '/sharing', key: 'nav.sharing', Icon: ShareIcon },
      { to: '/members', key: 'nav.members', Icon: MembersIcon },
      { to: '/import-export', key: 'nav.importExport', Icon: TransferIcon },
      { to: '/account', key: 'nav.account', Icon: AccountIcon },
      { to: '/admin/users', key: 'nav.accounts', Icon: AccountsIcon, adminOnly: true },
    ],
  },
]

/** The phone bar's links, in order; a fifth slot opens the More menu. */
export const phoneItems: NavItem[] = [homeItem, ...navGroups.flatMap((g) => g.items.filter((i) => i.phone))]
