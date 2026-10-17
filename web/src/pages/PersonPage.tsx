import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import { ApiError } from '../api/client'
import { deletePerson, getChecks, getPerson } from '../api/endpoints'
import type { ChildRelation, Family, PersonDetail, PersonRef, Relation } from '../api/types'
import { Button } from '../components/Button'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { PageHeading } from '../components/PageHeading'
import { useShortcuts } from '../hooks/useShortcuts'
import { fullName, lifespan } from '../lib/people'
import { EventDialog, type EventOwner } from '../people/EventDialog'
import { FamilyDialog } from '../people/FamilyDialog'
import { otherPartner } from '../people/familyLabel'
import { PersonDialog } from '../people/PersonDialog'
import { RelativeDialog } from '../people/RelativeDialog'
import { Timeline } from '../people/Timeline'
import { timelineItems, type TimelineItem } from '../people/timelineItems'
import { NotFound } from './NotFound'
import { CitationText } from '../sources/CitationEditor'
import { Avatar } from '../media/Avatar'
import { Gallery } from '../media/Gallery'
import { UploadButton } from '../media/UploadButton'
import { FindingList } from '../quality/FindingList'
import { ResearchPanel } from '../research/ResearchPanel'
import { PersonHeirlooms } from '../heirlooms/PersonHeirlooms'

type Open =
  | { kind: 'edit' }
  | { kind: 'delete' }
  | { kind: 'relative'; relation: Relation }
  | { kind: 'event'; owner: EventOwner; item?: TimelineItem }
  | { kind: 'family'; family: Family }
  | null

export function PersonPage() {
  const { t } = useTranslation()
  const id = Number(useParams().id)
  const person = useQuery({ queryKey: ['person', id], queryFn: ({ signal }) => getPerson(id, signal) })
  const [open, setOpen] = useState<Open>(null)
  const checks = useQuery({
    queryKey: ['person', id, 'checks'],
    queryFn: ({ signal }) => getChecks(id, signal),
    enabled: person.isSuccess,
  })

  useShortcuts(
    {
      e: () => setOpen({ kind: 'edit' }),
      a: () => setOpen({ kind: 'event', owner: { personId: id } }),
      p: () => setOpen({ kind: 'relative', relation: 'parent' }),
      s: () => setOpen({ kind: 'relative', relation: 'partner' }),
      c: () => setOpen({ kind: 'relative', relation: 'child' }),
      b: () => setOpen({ kind: 'relative', relation: 'sibling' }),
    },
    person.isSuccess,
  )

  if (person.isError) {
    if (person.error instanceof ApiError && person.error.status === 404) return <NotFound />
    return (
      <p role="alert" className="font-medium text-red-700 dark:text-red-400">
        {t('common.loadFailed')}
      </p>
    )
  }
  if (person.isPending) {
    return (
      <p role="status" className="text-slate-600 dark:text-slate-400">
        {t('app.loading')}
      </p>
    )
  }
  const p = person.data
  const name = fullName(p) ?? t('person.unknown')
  const close = () => setOpen(null)

  return (
    <article className="max-w-4xl space-y-8">
      <header className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex items-center gap-4">
            <Avatar person={p} size={96} />
            <div>
            <PageHeading title={name}>{name}</PageHeading>
            <p className="mt-1 text-slate-700 dark:text-slate-300">
              {[t(`sex.${p.sex}`), lifespan(summary(p)), p.living ? t('person.living') : t('person.deceased')]
                .filter(Boolean)
                .join(' · ')}
            </p>
            {p.nickname && <p className="text-slate-700 dark:text-slate-300">{t('person.nicknameIs', { nickname: p.nickname })}</p>}
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" onClick={() => setOpen({ kind: 'edit' })}>
              {t('common.edit')}
              <span className="sr-only"> ({t('shortcuts.key', { key: 'e' })})</span>
            </Button>
            <Link
              to={`/tree?root=${p.id}`}
              className="inline-flex min-h-11 items-center rounded-lg px-3 font-medium text-brand-700 underline dark:text-brand-100"
            >
              {t('person.viewInTree')}
            </Link>
            <Link
              to={`/relationship?a=${p.id}`}
              className="inline-flex min-h-11 items-center rounded-lg px-3 font-medium text-brand-700 underline dark:text-brand-100"
            >
              {t('person.relationshipLink')}
            </Link>
            <Button variant="ghost" onClick={() => setOpen({ kind: 'delete' })}>
              {t('person.delete')}
            </Button>
          </div>
        </div>
        {p.alternateNames.length > 0 && (
          <ul className="flex flex-wrap gap-2" aria-label={t('person.alternateNames')}>
            {p.alternateNames.map((n) => (
              <li key={n.id} className="rounded-full bg-slate-100 px-3 py-1 text-sm dark:bg-slate-800">
                {t(`nameType.${n.type}`)}: {[n.givenNames, n.surname].filter(Boolean).join(' ')}
              </li>
            ))}
          </ul>
        )}
      </header>

      {checks.data && checks.data.findings.length > 0 && (
        <Section title={t('quality.forPerson')}>
          <FindingList report={checks.data} currentPersonId={p.id} />
        </Section>
      )}

      <Section
        title={t('person.events')}
        actions={
          <Button variant="secondary" onClick={() => setOpen({ kind: 'event', owner: { personId: p.id } })}>
            + {t('event.add')}
            <span className="sr-only"> ({t('shortcuts.key', { key: 'a' })})</span>
          </Button>
        }
      >
        <Timeline
          items={timelineItems(p.events, p.partnerFamilies)}
          personId={p.id}
          onEdit={(item) =>
            setOpen({
              kind: 'event',
              item,
              owner: item.event.familyId !== null ? { familyId: item.event.familyId } : { personId: item.event.personId! },
            })
          }
        />
      </Section>

      <Section
        title={t('person.parentsAndSiblings')}
        actions={
          <>
            <Button variant="secondary" onClick={() => setOpen({ kind: 'relative', relation: 'parent' })}>
              + {t('relative.parent')}
              <span className="sr-only"> ({t('shortcuts.key', { key: 'p' })})</span>
            </Button>
            <Button variant="secondary" onClick={() => setOpen({ kind: 'relative', relation: 'sibling' })}>
              + {t('relative.sibling')}
              <span className="sr-only"> ({t('shortcuts.key', { key: 'b' })})</span>
            </Button>
          </>
        }
      >
        {p.parentFamilies.length === 0 && <p className="text-slate-600 dark:text-slate-400">{t('person.noParents')}</p>}
        {p.parentFamilies.map((f) => {
          const self = f.children.find((c) => c.person.id === p.id)
          const siblings = f.children.filter((c) => c.person.id !== p.id)
          return (
            <FamilyCard key={f.id} family={f} onEdit={() => setOpen({ kind: 'family', family: f })}>
              <h3 className="font-semibold">{t('person.parents')}</h3>
              <ul className="mt-1 space-y-1">
                <li>
                  <PersonLink person={f.partner1} />
                  {self && <RelationNote relation={self.relationPartner1} />}
                </li>
                <li>
                  <PersonLink person={f.partner2} />
                  {self && <RelationNote relation={self.relationPartner2} />}
                </li>
              </ul>
              {siblings.length > 0 && (
                <>
                  <h3 className="mt-3 font-semibold">{t('person.siblings')}</h3>
                  <ul className="mt-1 space-y-1">
                    {siblings.map((c) => (
                      <li key={c.person.id}>
                        <PersonLink person={c.person} />
                      </li>
                    ))}
                  </ul>
                </>
              )}
            </FamilyCard>
          )
        })}
      </Section>

      <Section
        title={t('person.partnersAndChildren')}
        actions={
          <>
            <Button variant="secondary" onClick={() => setOpen({ kind: 'relative', relation: 'partner' })}>
              + {t('relative.partner')}
              <span className="sr-only"> ({t('shortcuts.key', { key: 's' })})</span>
            </Button>
            <Button variant="secondary" onClick={() => setOpen({ kind: 'relative', relation: 'child' })}>
              + {t('relative.child')}
              <span className="sr-only"> ({t('shortcuts.key', { key: 'c' })})</span>
            </Button>
          </>
        }
      >
        {p.partnerFamilies.length === 0 && <p className="text-slate-600 dark:text-slate-400">{t('person.noPartners')}</p>}
        {p.partnerFamilies.map((f) => {
          const side = f.partner1?.id === p.id ? 'relationPartner1' : 'relationPartner2'
          return (
            <FamilyCard key={f.id} family={f} onEdit={() => setOpen({ kind: 'family', family: f })}>
              <h3 className="font-semibold">
                <PersonLink person={otherPartner(f, p.id)} />
                <span className="ml-2 text-sm font-normal text-slate-600 dark:text-slate-400">{t(`unionType.${f.unionType}`)}</span>
              </h3>
              {f.children.length > 0 ? (
                <ul className="mt-2 space-y-1" aria-label={t('person.children')}>
                  {f.children.map((c) => (
                    <li key={c.person.id}>
                      <PersonLink person={c.person} />
                      <RelationNote relation={c[side]} />
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">{t('person.noChildren')}</p>
              )}
            </FamilyCard>
          )
        })}
      </Section>

      <Section title={t('media.section')} actions={<UploadButton owner={{ entityType: 'person', entityId: p.id }} />}>
        <Gallery owner={{ entityType: 'person', entityId: p.id }} />
      </Section>

      <Section title={t('heirloom.forPerson')}>
        <PersonHeirlooms person={{ ...p, birthDate: '', deathDate: '' }} />
      </Section>

      <Section title={t('research.forPerson')}>
        <ResearchPanel person={{ id: p.id, label: name }} headingLevel={3} />
      </Section>

      {p.citations.length > 0 && (
        <Section title={t('citation.sourcesForPerson')}>
          <ul className="space-y-1">
            {p.citations.map((c) => (
              <li key={c.citationId}>
                <CitationText citation={c} />
              </li>
            ))}
          </ul>
        </Section>
      )}

      {p.notes && (
        <Section title={t('person.notes')}>
          <p className="whitespace-pre-line">{p.notes}</p>
        </Section>
      )}

      <PersonDialog open={open?.kind === 'edit'} onClose={close} person={p} onSaved={close} />
      <DeletePerson open={open?.kind === 'delete'} onClose={close} person={p} name={name} />
      {open?.kind === 'relative' && <RelativeDialog open anchor={p} relation={open.relation} onClose={close} onSaved={close} />}
      {open?.kind === 'event' && (
        <EventDialog open onClose={close} owner={open.owner} event={open.item?.event} />
      )}
      {open?.kind === 'family' && <FamilyDialog open onClose={close} family={open.family} />}
    </article>
  )
}

/** Birth and death as the API's PersonRef would report them. */
function summary(p: PersonDetail) {
  const pick = (types: string[]) =>
    p.events.find((e) => !e.role && types.includes(e.type) && e.status !== 'disproven')?.date
  const b = pick(['BIRT']) ?? pick(['CHR', 'BAPM'])
  const d = pick(['DEAT']) ?? pick(['BURI', 'CREM'])
  return { birthDate: b ? b.normalized || b.raw : '', deathDate: d ? d.normalized || d.raw : '' }
}

function Section({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  const id = useId()
  return (
    <section aria-labelledby={id} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-200 pb-2 dark:border-slate-800">
        <h2 id={id} className="text-xl font-semibold">
          {title}
        </h2>
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
      </div>
      {children}
    </section>
  )
}

function FamilyCard({ children, onEdit, family }: { children: ReactNode; onEdit: () => void; family: Family }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-start justify-between gap-3 rounded-xl border border-slate-200 p-4 dark:border-slate-800">
      <div className="min-w-0">
        {children}
        {family.citations.length > 0 && (
          <ul className="mt-2 text-sm" aria-label={t('citation.sourcesForFamily')}>
            {family.citations.map((c) => (
              <li key={c.citationId}>
                <span aria-hidden="true">📄 </span>
                <CitationText citation={c} />
              </li>
            ))}
          </ul>
        )}
      </div>
      <Button variant="ghost" onClick={onEdit}>
        {t('family.editShort')}
      </Button>
    </div>
  )
}

function PersonLink({ person }: { person: PersonRef | null }) {
  const { t } = useTranslation()
  if (!person) return <span className="italic text-slate-600 dark:text-slate-400">{t('person.unknownParent')}</span>
  return (
    <>
      <Link to={`/people/${person.id}`} className="font-medium text-brand-700 underline underline-offset-4 dark:text-brand-100">
        {fullName(person) ?? t('person.unknown')}
      </Link>
      {lifespan(person) && <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">{lifespan(person)}</span>}
    </>
  )
}

function RelationNote({ relation }: { relation: ChildRelation }) {
  const { t } = useTranslation()
  if (relation === 'birth') return null
  return <span className="ml-2 text-sm text-slate-600 dark:text-slate-400">({t(`childRelation.${relation}`)})</span>
}

function DeletePerson({ open, onClose, person, name }: { open: boolean; onClose: () => void; person: PersonDetail; name: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const remove = useMutation({
    mutationFn: () => deletePerson(person.id),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: ['person', person.id] })
      void queryClient.invalidateQueries({ queryKey: ['persons'] })
      void queryClient.invalidateQueries({ queryKey: ['person'] })
      navigate('/people')
    },
  })
  return (
    <ConfirmDialog
      open={open}
      title={t('person.delete')}
      message={t('person.deleteConfirm', { name })}
      confirmLabel={t('common.delete')}
      busy={remove.isPending}
      error={remove.error ? t('common.deleteFailed', { message: remove.error.message }) : null}
      onConfirm={() => remove.mutate()}
      onClose={onClose}
    />
  )
}
