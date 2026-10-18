import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GopherCredit } from './GopherCredit'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { ApiError, getAuthState } from '../api/client'
import { useLogout } from '../auth/useLogout'
import { TreeSwitcher } from '../trees/TreeSwitcher'
import { mdQuery, useMediaQuery } from '../hooks/useMediaQuery'
import { useShortcuts } from '../hooks/useShortcuts'
import { Button } from './Button'
import { ShortcutsHelp } from './ShortcutsHelp'
import { LanguageSwitch } from './LanguageSwitch'
import { ThemeSwitch } from './ThemeSwitch'
import { UpdateNotice } from './UpdateNotice'
import { DialogShell } from './DialogShell'
import { MoreIcon } from './icons'
import { homeItem, navGroups, phoneItems, type NavItem } from './navGroups'

function sideLinkClass({ isActive }: { isActive: boolean }) {
  return [
    'flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm font-medium',
    isActive
      ? 'bg-brand-50 text-brand-700 dark:bg-slate-800 dark:text-brand-100'
      : 'text-slate-600 hover:bg-slate-200 hover:text-slate-900 dark:text-slate-300 dark:hover:bg-slate-900 dark:hover:text-white',
  ].join(' ')
}

function SideLink({ item, onNavigate }: { item: NavItem; onNavigate?: () => void }) {
  const { t } = useTranslation()
  const { to, key, Icon, end } = item
  return (
    <NavLink to={to} end={end} onClick={onNavigate} className={sideLinkClass}>
      <Icon />
      <span className="leading-tight">{t(key)}</span>
    </NavLink>
  )
}

/** The links this user may see: administrator-only ones are left out for everyone else. */
function useVisibleGroups() {
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  const admin = auth.data?.user?.role === 'admin'
  return navGroups.map((g) => ({ ...g, items: g.items.filter((i) => admin || !i.adminOnly) }))
}

/** The sections as headed lists; the sidebar and the phone's More menu share it. */
function GroupedLinks({ idPrefix, level, onNavigate }: { idPrefix: string; level: 2 | 3; onNavigate?: () => void }) {
  const { t } = useTranslation()
  const groups = useVisibleGroups()
  const Heading = `h${level}` as const
  return (
    <>
      {groups.map((group) => {
        const headingId = `${idPrefix}-${group.id}`
        return (
          <div key={group.id} className="mt-3">
            <Heading id={headingId} className="px-3 pb-1 text-xs font-semibold tracking-wide text-slate-600 uppercase dark:text-slate-400">
              {t(group.key)}
            </Heading>
            <ul aria-labelledby={headingId} className="space-y-0.5">
              {group.items.map((item) => (
                <li key={item.to}>
                  <SideLink item={item} onNavigate={onNavigate} />
                </li>
              ))}
            </ul>
          </div>
        )
      })}
    </>
  )
}

/** The phone bar's fifth slot: a sheet with every section, grouped like the sidebar. */
function MoreMenu({ pathname }: { pathname: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const inMenu = useVisibleGroups().some((g) =>
    g.items.some((i) => !i.phone && (pathname === i.to || pathname.startsWith(`${i.to}/`))),
  )
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-current={inMenu ? 'true' : undefined}
        onClick={() => setOpen(true)}
        className={[
          'flex min-h-14 w-full flex-col items-center justify-center gap-0.5 px-1 text-xs font-medium',
          inMenu ? 'text-brand-700 dark:text-brand-100' : 'text-slate-600 hover:text-slate-900 dark:text-slate-300 dark:hover:text-white',
        ].join(' ')}
      >
        <MoreIcon />
        <span className={inMenu ? 'leading-tight underline underline-offset-4' : 'leading-tight'}>{t('nav.more')}</span>
      </button>
      {/* The native dialog provides Escape, the focus trap and focus return to the button. */}
      <DialogShell open={open} onClose={() => setOpen(false)} title={t('nav.more')}>
        <nav aria-label={t('nav.moreNavigation')}>
          <GroupedLinks idPrefix="more" level={3} onNavigate={() => setOpen(false)} />
        </nav>
      </DialogShell>
    </>
  )
}

export function Layout() {
  const { t } = useTranslation()
  const location = useLocation()
  const mainRef = useRef<HTMLElement>(null)
  const firstRender = useRef(true)
  const auth = useQuery({ queryKey: ['auth'], queryFn: ({ signal }) => getAuthState(signal), staleTime: Infinity })
  const logoutMutation = useLogout()
  const user = auth.data?.user
  const navigate = useNavigate()
  const [help, setHelp] = useState(false)
  const wide = useMediaQuery(mdQuery)
  const onPeople = location.pathname === '/people'

  // The people page handles "/" and "n" itself; elsewhere they go there.
  useShortcuts({
    '?': () => setHelp(true),
    ...(onPeople
      ? {}
      : {
          '/': () => {
            navigate('/people')
            setTimeout(() => document.getElementById('people-search')?.focus(), 50)
          },
          n: () => navigate('/people?new=1'),
        }),
  })

  // Screen readers get no page-load event on client-side navigation, so focus
  // goes to the new page's heading; on the initial load it stays put.
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      return
    }
    const heading = mainRef.current?.querySelector<HTMLElement>('h1')
    ;(heading ?? mainRef.current)?.focus()
  }, [location.pathname])

  return (
    <div className="min-h-dvh bg-slate-50 text-slate-900 print:min-h-0 dark:bg-slate-950 dark:text-slate-100">
      <a
        href="#main"
        className="sr-only print:hidden focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-brand-700 focus:px-4 focus:py-3 focus:text-white"
      >
        {t('app.skipToContent')}
      </a>

      <header className="sticky top-0 z-30 flex print:hidden h-14 items-center gap-3 border-b border-slate-200 bg-slate-100/95 px-4 backdrop-blur dark:border-slate-800 dark:bg-slate-950/95">
        <img src="/icons/logo-64.png" alt="" width={32} height={32} className="size-8 rounded-md" />
        <span className="max-sm:sr-only text-lg font-semibold text-brand-700 dark:text-brand-100">{t('app.name')}</span>
        <TreeSwitcher />
        {user && (
          <div className="ml-auto flex items-center gap-2 sm:gap-3">
            <LanguageSwitch />
            <ThemeSwitch />
            <span className="sr-only">{t('app.signedInAs', { name: user.displayName || user.username })}</span>
            <span aria-hidden="true" className="hidden text-sm text-slate-600 sm:inline dark:text-slate-400">
              {user.displayName || user.username}
            </span>
            {logoutMutation.isError &&
              !(logoutMutation.error instanceof ApiError && logoutMutation.error.status === 401) && (
                <span role="alert" className="text-sm font-medium text-red-700 dark:text-red-400">
                  {t('app.logOutFailed')}
                </span>
              )}
            <Button variant="secondary" onClick={() => logoutMutation.mutate()} busy={logoutMutation.isPending}>
              {t('app.logOut')}
            </Button>
          </div>
        )}
      </header>

      <div className="md:flex">
        {wide ? (
          <nav
            aria-label={t('app.mainNavigation')}
            className="sticky top-14 h-[calc(100dvh-3.5rem)] w-60 shrink-0 overflow-y-auto border-r border-slate-200 bg-slate-100 p-3 print:hidden dark:border-slate-800 dark:bg-slate-950"
          >
            <SideLink item={homeItem} />
            <GroupedLinks idPrefix="side" level={2} />
            <GopherCredit className="px-3 pt-4 pb-3" />
          </nav>
        ) : (
          <nav
            aria-label={t('app.mainNavigation')}
            className="fixed inset-x-0 bottom-0 z-30 border-t border-slate-200 bg-slate-100 pb-[env(safe-area-inset-bottom)] print:hidden dark:border-slate-800 dark:bg-slate-950"
          >
            <ul className="grid grid-cols-5">
              {phoneItems.map(({ to, key, Icon, end }) => (
                <li key={to}>
                  <NavLink
                    to={to}
                    end={end}
                    className={({ isActive }) =>
                      [
                        'flex min-h-14 flex-col items-center justify-center gap-0.5 px-1 text-xs font-medium',
                        isActive ? 'text-brand-700 dark:text-brand-100' : 'text-slate-600 hover:text-slate-900 dark:text-slate-300 dark:hover:text-white',
                      ].join(' ')
                    }
                  >
                    <Icon />
                    <span className="text-center leading-tight">{t(key)}</span>
                  </NavLink>
                </li>
              ))}
              <li>
                <MoreMenu pathname={location.pathname} />
              </li>
            </ul>
          </nav>
        )}

        <main
          id="main"
          ref={mainRef}
          tabIndex={-1}
          className="min-w-0 flex-1 px-4 pt-6 pb-24 focus:outline-none md:px-8 md:pb-8 print:p-0"
        >
          <Outlet />
        </main>
      </div>
      <ShortcutsHelp open={help} onClose={() => setHelp(false)} />
      <UpdateNotice />
    </div>
  )
}
