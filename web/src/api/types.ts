export type Sex = 'M' | 'F' | 'U' | 'X'
export type FactStatus = 'accepted' | 'disputed' | 'disproven'
export type UnionType = 'married' | 'partners' | 'unknown'
export type ChildRelation = 'birth' | 'adopted' | 'foster' | 'step' | 'surrogate' | 'sealing' | 'unknown'
export type NameType = 'birth' | 'married' | 'aka' | 'religious' | 'immigrant' | 'other'
export type Relation = 'parent' | 'partner' | 'child' | 'sibling'

export interface PersonRef {
  id: number
  givenNames: string
  surname: string
  sex: Sex
  birthDate: string
  deathDate: string
  living: boolean
  portrait: PortraitRef | null
}

export interface AlternateName {
  id: number
  type: NameType
  givenNames: string
  surname: string
  namePrefix: string
  nameSuffix: string
  nickname: string
  status: FactStatus
  statusReason: string
  sortOrder: number
  citations: CitationRef[]
}

export interface Person {
  id: number
  givenNames: string
  surname: string
  namePrefix: string
  nameSuffix: string
  nickname: string
  sex: Sex
  isLiving: boolean | null
  living: boolean
  notes: string
  alternateNames: AlternateName[]
  citations: CitationRef[]
  portrait: PortraitRef | null
  createdAt: string
  updatedAt: string
}

export interface DateValue {
  raw: string
  normalized: string
  qualifier: string
  valid: boolean
  sortKey: number | null
}

export interface PlaceRef {
  id: number
  fullName: string
}

export interface Participant {
  person: PersonRef
  role: string
  customRole: string
  notes: string
}

export interface LifeEvent {
  id: number
  personId: number | null
  familyId: number | null
  type: string
  customLabel: string
  date: DateValue
  place: PlaceRef | null
  description: string
  notes: string
  status: FactStatus
  statusReason: string
  sortOrder: number
  participants: Participant[]
  citations: CitationRef[]
  role?: string
  createdAt: string
  updatedAt: string
}

export interface ChildLink {
  person: PersonRef
  relationPartner1: ChildRelation
  relationPartner2: ChildRelation
  sortOrder: number
}

export interface Family {
  id: number
  partner1: PersonRef | null
  partner2: PersonRef | null
  unionType: UnionType
  notes: string
  children: ChildLink[]
  events: LifeEvent[]
  citations: CitationRef[]
  createdAt: string
  updatedAt: string
}

export interface PersonDetail extends Person {
  events: LifeEvent[]
  parentFamilies: Family[]
  partnerFamilies: Family[]
}

export interface PersonList {
  items: PersonRef[]
  total: number
}

export interface Place {
  id: number
  parentId: number | null
  name: string
  placeType: string
  lat: number | null
  lng: number | null
  notes: string
  fullName: string
}

export interface AlternateNameInput {
  id?: number
  type: NameType
  givenNames: string
  surname: string
  namePrefix: string
  nameSuffix: string
  nickname: string
  status: FactStatus
  statusReason: string
}

/** Date and place of a birth or death entered together with a new person. */
export interface LifeEventInput {
  date: string
  placeId: number | null
}

export interface PersonInput {
  givenNames: string
  surname: string
  namePrefix: string
  nameSuffix: string
  nickname: string
  sex: Sex
  isLiving: boolean | null
  notes: string
  alternateNames: AlternateNameInput[]
  /** Only used when creating a person. */
  birth?: LifeEventInput
  death?: LifeEventInput
  addCitations?: NewCitation[]
  removeCitations?: number[]
}

export interface ParticipantInput {
  personId: number
  role: string
  customRole: string
  notes: string
}

export interface EventInput {
  personId: number | null
  familyId: number | null
  type: string
  customLabel: string
  date: string
  placeId: number | null
  description: string
  notes: string
  status: FactStatus
  statusReason: string
  sortOrder: number
  participants: ParticipantInput[]
  addCitations?: NewCitation[]
  removeCitations?: number[]
}

export interface FamilyInput {
  partner1Id: number | null
  partner2Id: number | null
  unionType: UnionType
  notes: string
}

export interface RelativeInput {
  relation: Relation
  personId?: number
  person?: PersonInput
  familyId?: number
  unionType?: UnionType
  childRelation?: ChildRelation
  citation?: NewCitation
}

export interface RelativeResult {
  person: Person
  family: Family
}

export interface PlaceInput {
  parentId: number | null
  name: string
  placeType: string
  lat: number | null
  lng: number | null
  notes: string
}

export interface ParsedDate {
  valid: boolean
  empty: boolean
  normalized: string
  qualifier: string
  error?: string
}

export interface TreeChild {
  personId: number
  relationPartner1: ChildRelation
  relationPartner2: ChildRelation
}

export interface TreeFamily {
  id: number
  partner1Id: number | null
  partner2Id: number | null
  unionType: UnionType
  children: TreeChild[]
}

export interface TreeGraph {
  rootId: number
  persons: Record<string, PersonRef>
  families: TreeFamily[]
  truncated: boolean
}

export interface CitationRef {
  citationId: number
  sourceId: number
  sourceTitle: string
  page: string
  quality: number | null
  field: string
  status: FactStatus
}

export interface NewCitation {
  sourceId: number
  page: string
  quality: number | null
  text: string
}

export interface Repository {
  id: number
  name: string
  address: string
  url: string
  notes: string
}

export interface Source {
  id: number
  title: string
  author: string
  publication: string
  callNumber: string
  repository: Repository | null
  notes: string
  citationCount: number
}

export interface SourceInput {
  title: string
  author: string
  publication: string
  callNumber: string
  repositoryId: number | null
  notes: string
}

export interface CitationLink {
  entityType: 'person' | 'event' | 'family' | 'name'
  entityId: number
  field: string
  status: FactStatus
  statusReason: string
  /** "Anna Müller", or "BIRT|Anna Müller" / "EVEN:Custom|Anna Müller" for events. */
  label: string
  personId: number | null
}

export interface Citation {
  id: number
  sourceId: number
  page: string
  quality: number | null
  text: string
  notes: string
  links: CitationLink[]
}

export interface SourceDetail extends Source {
  citations: Citation[]
}

export interface PortraitRef {
  mediaId: number
  regionId: number | null
}

export type MediaKind = 'image' | 'document' | 'audio' | 'video'

export interface MediaRef {
  id: number
  kind: MediaKind
  mime: string
  title: string
  width: number | null
  height: number | null
}

export interface MediaRegion {
  id: number
  person: PersonRef | null
  name: string
  x: number
  y: number
  w: number
  h: number
  source: 'manual' | 'xmp'
}

export interface MediaLink {
  entityType: 'person' | 'event' | 'family' | 'source' | 'heirloom'
  entityId: number
  label: string
  personId: number | null
}

export interface Media extends MediaRef {
  size: number
  originalName: string
  date: string
  description: string
  transcript: string
  takenAt: string
  lat: number | null
  lng: number | null
  links: MediaLink[]
  regions: MediaRegion[]
}

export interface RegionInput {
  personId: number | null
  name: string
  x: number
  y: number
  w: number
  h: number
}

export interface TagStat {
  path: string
  count: number
  outcome: 'mapped' | 'kept' | 'dropped'
  reason?: string
}

export interface ImportReport {
  version: string
  encoding: string
  source: string
  counts: Record<string, number>
  tags: TagStat[]
  warnings: { line: number; message: string }[]
  invalidDates: number
  invalidDateExamples: string[]
  brokenReferences: string[]
  durationMs: number
}

export interface VerifyReport {
  version: string
  ok: boolean
  rows: { kind: string; tree: number; roundTrip: number }[]
  warnings: { line: number; message: string }[]
  dropped: TagStat[]
}

export type CheckRule =
  | 'birth_after_death'
  | 'burial_before_death'
  | 'event_before_birth'
  | 'event_after_death'
  | 'too_old'
  | 'living_too_old'
  | 'parent_too_young'
  | 'mother_too_old'
  | 'born_after_mother_death'
  | 'born_after_father_death'
  | 'married_before_birth'
  | 'married_after_death'
  | 'invalid_date'

export interface Finding {
  /** Identifies the finding; a research task made from it carries it. */
  origin: string
  rule: CheckRule
  severity: 'error' | 'warning'
  personId: number
  otherPersonId?: number
  familyId?: number
  eventId?: number
  eventType?: string
  years?: number
}

export interface CheckReport {
  findings: Finding[]
  persons: Record<number, PersonRef>
  /** Origins of findings that became research tasks, with the task id. */
  taskIds: Record<string, number>
}

export interface DateProposal {
  eventId: number
  type: string
  customLabel: string
  personId: number | null
  familyId: number | null
  personIds: number[]
  raw: string
  proposed: string
  wasValid: boolean
}

export interface DateProposals {
  items: DateProposal[]
  unreadable: number
  persons: Record<number, PersonRef>
}

export interface DateChange {
  eventId: number
  raw: string
  date: string
}

export interface Kinship {
  up: number
  down: number
  half: boolean
  adoptive: boolean
  /** Negative ids stand for the unknown parents of a family. */
  ancestors: number[]
  path: number[]
}

export type RelationshipKind = 'self' | 'blood' | 'spouse' | 'spouse_of_relative' | 'relative_of_spouse' | 'none'

export interface RelationshipReport {
  a: number
  b: number
  kind: RelationshipKind
  kinship?: Kinship
  via?: number
  others: Kinship[]
  persons: Record<number, PersonRef>
}

export type ShareScope = 'tree' | 'descendants'
export type SharePrivacy = 'deceased' | 'living_names'

export interface ShareLink {
  id: number
  label: string
  scope: ShareScope
  root: PersonRef | null
  privacy: SharePrivacy
  expiresAt: string | null
  revokedAt: string | null
  lastUsedAt: string | null
  createdAt: string
  /** Only present right after creating the link. */
  token?: string
}

export interface ShareLinkInput {
  label: string
  scope: ShareScope
  rootPersonId: number | null
  privacy: SharePrivacy
  expiresOn: string
}

export interface ShareInfo {
  treeName: string
  label: string
  scope: ShareScope
  privacy: SharePrivacy
  root: PersonRef | null
  startId: number | null
}

export interface DayEvent {
  eventId: number
  type: 'BIRT' | 'MARR' | 'DEAT'
  year: number
  personIds: number[]
}

export interface DayReport {
  events: DayEvent[]
  persons: Record<number, PersonRef>
}

export interface CalendarFeed {
  id: number
  label: string
  includeLiving: boolean
  revokedAt: string | null
  lastUsedAt: string | null
  createdAt: string
  /** Only present right after creating the feed. */
  token?: string
}

export type ResearchEntity = 'person' | 'source' | 'place'
export type TaskStatus = 'open' | 'in_progress' | 'done'
export type TaskPriority = 'low' | 'normal' | 'high'
export type SearchResult = 'found' | 'not_found' | 'partial'

export interface ResearchLink {
  entityType: ResearchEntity
  entityId: number
  label: string
}

export type ResearchLinkInput = Omit<ResearchLink, 'label'>

export interface ResearchTask {
  id: number
  title: string
  status: TaskStatus
  priority: TaskPriority
  dueOn: string
  notes: string
  origin: string
  doneAt: string | null
  links: ResearchLink[]
  logCount: number
  createdAt: string
  updatedAt: string
}

export interface ResearchTaskInput {
  title: string
  status: TaskStatus
  priority: TaskPriority
  dueOn: string
  notes: string
  origin: string
  links: ResearchLinkInput[]
}

export interface LogEntry {
  id: number
  taskId: number | null
  taskTitle: string
  searchedOn: string
  query: string
  location: string
  result: SearchResult
  notes: string
  links: ResearchLink[]
  createdAt: string
  updatedAt: string
}

export interface LogEntryInput {
  taskId: number | null
  searchedOn: string
  query: string
  location: string
  result: SearchResult
  notes: string
  links: ResearchLinkInput[]
}

export interface Suggestion {
  origin: string
  kind: 'missing_birth' | 'birth_unsourced' | 'missing_death' | 'finding'
  personId: number
  finding?: Finding
}

export interface SuggestionReport {
  suggestions: Suggestion[]
  persons: Record<number, PersonRef>
}

export type RoleKind = 'principal' | 'partner' | 'parent' | 'participant'
export type ColumnKind = 'given' | 'surname' | 'sex' | 'age' | 'occupation' | 'residence' | 'birthplace' | 'notes'

export interface TemplateRole {
  key: string
  label: string
  kind: RoleKind
  participant: string
}

export interface RecordTemplate {
  key: string
  name: string
  builtin: boolean
  eventType: string
  roles: TemplateRole[]
  columns: ColumnKind[]
}

export type RowAction = 'new' | 'person' | 'skip'

export interface TranscriptionRow {
  line: string
  role: string
  values: Partial<Record<ColumnKind, string>>
  action: RowAction
  personId: number | null
}

export interface Transcription {
  id: number
  templateKey: string
  title: string
  source: { id: number; title: string } | null
  page: string
  date: string
  place: PlaceRef | null
  notes: string
  rows: TranscriptionRow[]
  status: 'draft' | 'applied'
  eventId: number | null
  appliedAt: string | null
  persons: Record<number, PersonRef>
  createdAt: string
  updatedAt: string
}

export interface TranscriptionInput {
  templateKey: string
  title: string
  sourceId: number | null
  page: string
  date: string
  placeId: number | null
  notes: string
  rows: TranscriptionRow[]
}

export interface MatchCandidate {
  person: PersonRef
  score: number
}

export interface ApplyResult extends Transcription {
  eventId: number
  familyId?: number
  created: number[]
  facts: number
}

export type HeirloomKind = 'jewellery' | 'furniture' | 'document' | 'photo_album' | 'tool' | 'textile' | 'other'
export type CustodyHow = 'inherited' | 'gift' | 'purchased' | 'made' | 'found' | 'other'

export interface Custody {
  id: number
  person: PersonRef | null
  fromDate: string
  toDate: string
  how: CustodyHow
  notes: string
}

export interface CustodyInput {
  personId: number | null
  fromDate: string
  toDate: string
  how: CustodyHow
  notes: string
}

export interface Heirloom {
  id: number
  name: string
  kind: HeirloomKind
  description: string
  madeDate: string
  originPlace: PlaceRef | null
  currentLocation: string
  notes: string
  custody: Custody[]
  citations: CitationRef[]
  createdAt: string
  updatedAt: string
}

export interface HeirloomRef {
  id: number
  name: string
  kind: HeirloomKind
  madeDate: string
  holder: PersonRef | null
  photoId: number | null
}

export interface HeirloomInput {
  name: string
  kind: HeirloomKind
  description: string
  madeDate: string
  originPlaceId: number | null
  currentLocation: string
  notes: string
  custody: CustodyInput[]
  addCitations?: NewCitation[]
  removeCitations?: number[]
}

export interface Average {
  count: number
  average: number
}

export interface NameCount {
  name: string
  count: number
}

export interface Stats {
  persons: number
  families: number
  sources: number
  withLifespan: number
  lifespans: { decade: number; count: number; average: number; min: number; max: number }[]
  marriages: { decade: number; firstMen: Average; firstWomen: Average; later: Average }[]
  childrenHistogram: number[]
  childrenAverage: number
  surnames: NameCount[]
  givenNames: NameCount[]
  givenNameTrends: { decade: number; men: NameCount[]; women: NameCount[] }[]
}

export interface MapPlace {
  name: string
  lat: number
  lng: number
  approximate: boolean
}

export interface UnmappedPlace extends Place {
  events: number
}

export interface UnmappedPlaces {
  total: number
  places: UnmappedPlace[]
}

export interface MapTrack {
  personId: number
  points: { key: number; placeId: number; type: string }[]
  death: number
}

export interface MapData {
  tracks: MapTrack[]
  places: Record<number, MapPlace>
  persons: Record<number, PersonRef>
  unmapped: number
}
