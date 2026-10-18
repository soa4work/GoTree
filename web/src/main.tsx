import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { registerSW } from 'virtual:pwa-register'
import { App } from './App'
import './i18n'
import './index.css'
import { markUpdateReady } from './lib/updateReady'
import { createQueryClient } from './queryClient'

const queryClient = createQueryClient()

// A new version waits until the user chooses to reload; see UpdateNotice.
const updateSW = registerSW({
  immediate: true,
  onNeedRefresh: () => markUpdateReady(),
})
window.addEventListener('gotree:apply-update', () => void updateSW(true))

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
