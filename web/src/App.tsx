import { Route, Routes } from 'react-router'
import { AuthGate } from './auth/AuthGate'
import { Layout } from './components/Layout'
import { ChartPage } from './pages/ChartPage'
import { Dashboard } from './pages/Dashboard'
import { MapPage } from './pages/MapPage'
import { StatsPage } from './pages/StatsPage'
import { HeirloomPage } from './pages/HeirloomPage'
import { HeirloomsPage } from './pages/HeirloomsPage'
import { NotFound } from './pages/NotFound'
import { PeoplePage } from './pages/PeoplePage'
import { PersonPage } from './pages/PersonPage'
import { QualityPage } from './pages/QualityPage'
import { RelationshipPage } from './pages/RelationshipPage'
import { ResearchPage } from './pages/ResearchPage'
import { TranscribePage } from './pages/TranscribePage'
import { TranscriptionPage } from './pages/TranscriptionPage'
import { SharingPage } from './pages/SharingPage'
import { ShareApp } from './share/ShareApp'
import { AccountPage } from './pages/AccountPage'
import { ImportExportPage } from './pages/ImportExportPage'
import { MembersPage } from './pages/MembersPage'
import { MediaListPage } from './pages/MediaListPage'
import { MediaPage } from './pages/MediaPage'
import { SourcePage } from './pages/SourcePage'
import { SourcesPage } from './pages/SourcesPage'
import { TreePage } from './pages/TreePage'

export function App() {
  return (
    <Routes>
      {/* Visitors with a share link have no account, so this sits outside the gate. */}
      <Route path="share/:token/*" element={<ShareApp />} />
      <Route path="*" element={<AuthedApp />} />
    </Routes>
  )
}

function AuthedApp() {
  return (
    <AuthGate>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="people" element={<PeoplePage />} />
          <Route path="people/:id" element={<PersonPage />} />
          <Route path="tree" element={<TreePage />} />
          <Route path="chart" element={<ChartPage />} />
          <Route path="stats" element={<StatsPage />} />
          <Route path="map" element={<MapPage />} />
          <Route path="sources" element={<SourcesPage />} />
          <Route path="sources/:id" element={<SourcePage />} />
          <Route path="heirlooms" element={<HeirloomsPage />} />
          <Route path="heirlooms/:id" element={<HeirloomPage />} />
          <Route path="media" element={<MediaListPage />} />
          <Route path="media/:id" element={<MediaPage />} />
          <Route path="relationship" element={<RelationshipPage />} />
          <Route path="quality" element={<QualityPage />} />
          <Route path="research" element={<ResearchPage />} />
          <Route path="transcribe" element={<TranscribePage />} />
          <Route path="transcribe/new" element={<TranscriptionPage />} />
          <Route path="transcribe/:id" element={<TranscriptionPage />} />
          <Route path="sharing" element={<SharingPage />} />
          <Route path="members" element={<MembersPage />} />
          <Route path="import-export" element={<ImportExportPage />} />
          <Route path="account" element={<AccountPage />} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </AuthGate>
  )
}
