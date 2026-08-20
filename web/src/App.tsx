import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import { ToastProvider } from './components/Toast'
import Today from './pages/Today'
import Overview from './pages/Overview'
import Tasks from './pages/Tasks'
import TaskDetail from './pages/TaskDetail'
import Jobs from './pages/Jobs'
import Confirms from './pages/Confirms'
import Tools from './pages/Tools'
import Artifacts from './pages/Artifacts'
import Logs from './pages/Logs'
import Settings from './pages/Settings'
import Memories from './pages/Memories'
import Audit from './pages/Audit'
import Mcp from './pages/Mcp'
import Tokens from './pages/Tokens'
import Chat from './pages/Chat'
import Knowledge from './pages/Knowledge'
import Events from './pages/Events'
import TokenUsage from './pages/TokenUsage'
import Schedule from './pages/Schedule'
import Profiles from './pages/Profiles'

export default function App() {
  return (
    <ToastProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<Today />} />
            <Route path="overview" element={<Overview />} />
            <Route path="chat" element={<Chat />} />
            <Route path="tasks" element={<Tasks />} />
            <Route path="tasks/:id" element={<TaskDetail />} />
            <Route path="jobs" element={<Jobs />} />
            <Route path="confirms" element={<Confirms />} />
            <Route path="memories" element={<Memories />} />
            <Route path="mcp" element={<Mcp />} />
            <Route path="audit" element={<Audit />} />
            <Route path="tokens" element={<Tokens />} />
            <Route path="knowledge" element={<Knowledge />} />
            <Route path="tools" element={<Tools />} />
            <Route path="events" element={<Events />} />
            <Route path="token-usage" element={<TokenUsage />} />
            <Route path="schedule" element={<Schedule />} />
            <Route path="profiles" element={<Profiles />} />
            <Route path="artifacts" element={<Artifacts />} />
            <Route path="logs" element={<Logs />} />
            <Route path="settings" element={<Settings />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ToastProvider>
  )
}
