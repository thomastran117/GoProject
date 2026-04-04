import { useState } from 'react'
import SubmissionUpload from './components/SubmissionUpload'

function App() {
  const [assignmentId, setAssignmentId] = useState(1)
  const [token, setToken] = useState('')

  return (
    <main className="min-h-screen bg-gray-50 flex items-center justify-center p-8">
      <div className="w-full max-w-md space-y-6">
        <h1 className="text-2xl font-bold text-gray-900">Assignment Submission</h1>

        <div className="flex flex-col gap-3">
          <label className="text-sm font-medium text-gray-700">
            Assignment ID
            <input
              type="number"
              min={1}
              value={assignmentId}
              onChange={e => setAssignmentId(Number(e.target.value))}
              className="mt-1 block w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </label>
          <label className="text-sm font-medium text-gray-700">
            JWT Token
            <input
              type="text"
              value={token}
              onChange={e => setToken(e.target.value)}
              placeholder="Paste your access token here"
              className="mt-1 block w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </label>
        </div>

        <SubmissionUpload assignmentId={assignmentId} token={token} />
      </div>
    </main>
  )
}

export default App
