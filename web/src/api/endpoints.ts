import { del, getJSON, postJSON, putJSON, uploadFile } from './client'
import type {
  EventInput,
  Family,
  FamilyInput,
  LifeEvent,
  ParsedDate,
  Person,
  PersonDetail,
  PersonInput,
  PersonList,
  Place,
  PlaceInput,
  RelativeInput,
  RelativeResult,
  Repository,
  Source,
  SourceDetail,
  SourceInput,
  TreeGraph,
  Media,
  MediaRef,
  RegionInput,
  ImportReport,
  VerifyReport,
  CheckReport,
  DateChange,
  DateProposals,
  RelationshipReport,
  DayReport,
  ShareInfo,
  ShareLink,
  ShareLinkInput,
  CalendarFeed,
  LogEntry,
  LogEntryInput,
  ResearchTask,
  ResearchTaskInput,
  SuggestionReport,
  ApplyResult,
  MatchCandidate,
  RecordTemplate,
  Transcription,
  TranscriptionInput,
  TranscriptionRow,
  Heirloom,
  HeirloomInput,
  HeirloomRef,
  MapData,
  Stats,
  UnmappedPlace,
} from './types'

const qs = (params: Record<string, string | number>) =>
  new URLSearchParams(Object.entries(params).map(([k, v]) => [k, String(v)])).toString()

export const listPersons = (q: string, limit: number, offset: number, signal?: AbortSignal) =>
  getJSON<PersonList>(`/persons?${qs({ q, limit, offset })}`, signal)
export const getPerson = (id: number, signal?: AbortSignal) => getJSON<PersonDetail>(`/persons/${id}`, signal)
export const createPerson = (input: PersonInput) => postJSON<Person>('/persons', input)
export const updatePerson = (id: number, input: PersonInput) => putJSON<Person>(`/persons/${id}`, input)
export const deletePerson = (id: number) => del(`/persons/${id}`)
export const addRelative = (id: number, input: RelativeInput) =>
  postJSON<RelativeResult>(`/persons/${id}/relatives`, input)

export const createEvent = (input: EventInput) => postJSON<LifeEvent>('/events', input)
export const updateEvent = (id: number, input: EventInput) => putJSON<LifeEvent>(`/events/${id}`, input)
export const deleteEvent = (id: number) => del(`/events/${id}`)

export const updateFamily = (id: number, input: FamilyInput) => putJSON<Family>(`/families/${id}`, input)
export const deleteFamily = (id: number) => del(`/families/${id}`)
export const removeChild = (familyId: number, childId: number) => del(`/families/${familyId}/children/${childId}`)

export const listPlaces = (q: string, signal?: AbortSignal) => getJSON<Place[]>(`/places?${qs({ q, limit: 20 })}`, signal)
export const updatePlace = (id: number, input: PlaceInput) => putJSON<Place>(`/places/${id}`, input)
export const createPlace = (input: PlaceInput) => postJSON<Place>('/places', input)

export const parseDate = (q: string, signal?: AbortSignal) => getJSON<ParsedDate>(`/dates/parse?${qs({ q })}`, signal)

export const getTree = (id: number, opts: { up: number; down: number; siblings: boolean }, signal?: AbortSignal) =>
  getJSON<TreeGraph>(`/tree/${id}?${qs({ up: opts.up, down: opts.down, siblings: opts.siblings ? 1 : 0 })}`, signal)

export const listSources = (q: string, signal?: AbortSignal) => getJSON<Source[]>(`/sources?${qs({ q, limit: 100 })}`, signal)
export const getSource = (id: number, signal?: AbortSignal) => getJSON<SourceDetail>(`/sources/${id}`, signal)
export const createSource = (input: SourceInput) => postJSON<Source>('/sources', input)
export const updateSource = (id: number, input: SourceInput) => putJSON<Source>(`/sources/${id}`, input)
export const deleteSource = (id: number) => del(`/sources/${id}`)

export const listRepositories = (q: string, signal?: AbortSignal) =>
  getJSON<Repository[]>(`/repositories?${qs({ q })}`, signal)
export const createRepository = (name: string) =>
  postJSON<Repository>('/repositories', { name, address: '', url: '', notes: '' })

export type MediaOwner = { entityType: 'person' | 'event' | 'family' | 'source' | 'heirloom'; entityId: number }

export const listMedia = (owner: MediaOwner | null, signal?: AbortSignal) =>
  getJSON<MediaRef[]>(`/media${owner ? `?${qs(owner)}` : '?limit=200'}`, signal)
export const uploadMedia = (file: File, owner: MediaOwner | null) =>
  uploadFile<Media>(`/media${owner ? `?${qs(owner)}` : ''}`, file)
export const getMedia = (id: number, signal?: AbortSignal) => getJSON<Media>(`/media/${id}`, signal)
export const updateMedia = (id: number, input: { title: string; date: string; description: string; transcript: string }) =>
  putJSON<Media>(`/media/${id}`, input)
export const deleteMedia = (id: number) => del(`/media/${id}`)
export const linkMedia = (id: number, owner: MediaOwner) => postJSON<Media>(`/media/${id}/links`, owner)
export const unlinkMedia = (id: number, owner: MediaOwner) => del(`/media/${id}/links/${owner.entityType}/${owner.entityId}`)
export const addRegion = (id: number, input: RegionInput) => postJSON<Media>(`/media/${id}/regions`, input)
export const updateRegion = (regionId: number, input: RegionInput) => putJSON<Media>(`/media/regions/${regionId}`, input)
export const deleteRegion = (regionId: number) => del(`/media/regions/${regionId}`)
export const setPortrait = (personId: number, mediaId: number | null, regionId: number | null) =>
  putJSON<unknown>(`/persons/${personId}/portrait`, { mediaId, regionId })

export const mediaFileUrl = (id: number) => `/api/media/${id}/file`
export const thumbUrl = (id: number, size: 128 | 256 | 512 | 1024, regionId?: number | null) =>
  `/api/media/${id}/thumb?size=${size}${regionId ? `&region=${regionId}` : ''}`

export const importGedcom = (file: File, mode: 'empty' | 'replace') =>
  uploadFile<ImportReport>(`/import/gedcom?${qs({ mode })}`, file)

export type ExportVersion = '5.5.1' | '7.0'
export type Privacy = 'all' | 'exclude-living' | 'name-only'
export const exportUrl = (version: ExportVersion, privacy: Privacy, format: 'ged' | 'gedzip') =>
  `/api/export/gedcom?${qs({ version, privacy, format })}`
export const verifyExport = (version: ExportVersion) => getJSON<VerifyReport>(`/export/verify?${qs({ version })}`)

export const getChecks = (personId: number | null, signal?: AbortSignal) =>
  getJSON<CheckReport>(`/checks${personId ? `?${qs({ person: personId })}` : ''}`, signal)
export const getDateProposals = (signal?: AbortSignal) => getJSON<DateProposals>('/dates/proposals', signal)
export const normalizeDates = (changes: DateChange[]) =>
  postJSON<{ updated: number; skipped: number }>('/dates/normalize', { changes })
export const getRelationship = (a: number, b: number, signal?: AbortSignal) =>
  getJSON<RelationshipReport>(`/relationship?${qs({ a, b })}`, signal)

export const listShareLinks = (signal?: AbortSignal) => getJSON<ShareLink[]>('/share-links', signal)
export const createShareLink = (input: ShareLinkInput) => postJSON<ShareLink>('/share-links', input)
export const revokeShareLink = (id: number) => del(`/share-links/${id}`)
export const getOnThisDay = (month: number, day: number, signal?: AbortSignal) =>
  getJSON<DayReport>(`/onthisday?${qs({ month, day })}`, signal)

/** The read-only API of a share link; the token stands in for a session. */
export const shareApi = (token: string) => {
  const base = `/share/${encodeURIComponent(token)}`
  return {
    info: (signal?: AbortSignal) => getJSON<ShareInfo>(base, signal),
    persons: (q: string, signal?: AbortSignal) => getJSON<PersonList>(`${base}/persons?${qs({ q })}`, signal),
    person: (id: number, signal?: AbortSignal) => getJSON<PersonDetail>(`${base}/persons/${id}`, signal),
    tree: (id: number, opts: { up: number; down: number }, signal?: AbortSignal) =>
      getJSON<TreeGraph>(`${base}/tree/${id}?${qs(opts)}`, signal),
    onThisDay: (month: number, day: number, signal?: AbortSignal) =>
      getJSON<DayReport>(`${base}/onthisday?${qs({ month, day })}`, signal),
  }
}

export const listCalendarFeeds = (signal?: AbortSignal) => getJSON<CalendarFeed[]>('/calendar-feeds', signal)
export const createCalendarFeed = (input: { label: string; includeLiving: boolean }) =>
  postJSON<CalendarFeed>('/calendar-feeds', input)
export const revokeCalendarFeed = (id: number) => del(`/calendar-feeds/${id}`)

export type ResearchQuery = { status?: string; person?: number; task?: number; limit?: number }
const researchQs = (f: ResearchQuery) =>
  qs(Object.fromEntries(Object.entries(f).filter(([, v]) => v !== undefined && v !== '')) as Record<string, string | number>)

export const listTasks = (f: ResearchQuery, signal?: AbortSignal) => getJSON<ResearchTask[]>(`/research/tasks?${researchQs(f)}`, signal)
export const createTask = (input: ResearchTaskInput) => postJSON<ResearchTask>('/research/tasks', input)
export const updateTask = (id: number, input: ResearchTaskInput) => putJSON<ResearchTask>(`/research/tasks/${id}`, input)
export const deleteTask = (id: number) => del(`/research/tasks/${id}`)
export const listLog = (f: ResearchQuery, signal?: AbortSignal) => getJSON<LogEntry[]>(`/research/log?${researchQs(f)}`, signal)
export const createLogEntry = (input: LogEntryInput) => postJSON<LogEntry>('/research/log', input)
export const updateLogEntry = (id: number, input: LogEntryInput) => putJSON<LogEntry>(`/research/log/${id}`, input)
export const deleteLogEntry = (id: number) => del(`/research/log/${id}`)
export const getSuggestions = (personId: number, signal?: AbortSignal) =>
  getJSON<SuggestionReport>(`/research/suggestions?${qs({ person: personId })}`, signal)

export const listTemplates = (signal?: AbortSignal) => getJSON<RecordTemplate[]>('/templates', signal)
export const createTemplate = (t: RecordTemplate) => postJSON<RecordTemplate>('/templates', t)
export const updateTemplate = (id: number, t: RecordTemplate) => putJSON<RecordTemplate>(`/templates/${id}`, t)
export const deleteTemplate = (id: number) => del(`/templates/${id}`)
export const listTranscriptions = (signal?: AbortSignal) => getJSON<Transcription[]>('/transcriptions', signal)
export const getTranscription = (id: number, signal?: AbortSignal) => getJSON<Transcription>(`/transcriptions/${id}`, signal)
export const createTranscription = (input: TranscriptionInput) => postJSON<Transcription>('/transcriptions', input)
export const updateTranscription = (id: number, input: TranscriptionInput) => putJSON<Transcription>(`/transcriptions/${id}`, input)
export const deleteTranscription = (id: number) => del(`/transcriptions/${id}`)
export const applyTranscription = (id: number) => postJSON<ApplyResult>(`/transcriptions/${id}/apply`, {})
export const matchRows = (date: string, rows: TranscriptionRow[]) =>
  postJSON<MatchCandidate[][]>('/transcriptions/match', { date, rows })

export const listHeirlooms = (f: { q?: string; person?: number }, signal?: AbortSignal) =>
  getJSON<HeirloomRef[]>(`/heirlooms?${qs({ q: f.q ?? '', person: f.person ?? 0 })}`, signal)
export const getHeirloom = (id: number, signal?: AbortSignal) => getJSON<Heirloom>(`/heirlooms/${id}`, signal)
export const createHeirloom = (input: HeirloomInput) => postJSON<Heirloom>('/heirlooms', input)
export const updateHeirloom = (id: number, input: HeirloomInput) => putJSON<Heirloom>(`/heirlooms/${id}`, input)
export const deleteHeirloom = (id: number) => del(`/heirlooms/${id}`)

export const getStats = (signal?: AbortSignal) => getJSON<Stats>('/stats', signal)
export const getMapData = (scope: 'all' | 'ancestors' | 'descendants', root: number | null, signal?: AbortSignal) =>
  getJSON<MapData>(`/map?${qs({ scope, root: root ?? 0 })}`, signal)
export const getUnmappedPlaces = (signal?: AbortSignal) => getJSON<UnmappedPlace[]>('/map/unmapped-places', signal)
