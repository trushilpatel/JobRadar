import { useParams } from 'react-router-dom'

// Placeholder for Wave 2 Agent D.
//
// Mobile-only route component for `/jobs/:id`. Wave 2 will:
//   - Fetch the job by id (useQuery via a new useJob(id) hook)
//   - Render JobDetailPanel in `mode='route'` with a sticky action bar
//   - Restore scroll position on back-nav (React Router 7 default
//     scroll restoration usually covers this; verify on real device).
//
// Today it just confirms the route is mounted and the id flows through.

export function MobileJobDetailRoute() {
  const { id } = useParams<{ id: string }>()
  return (
    <div className="p-6 text-white">
      <div className="text-sm text-slate-400">Job detail placeholder</div>
      <div className="mt-1 font-mono text-xs text-slate-500">id={id}</div>
    </div>
  )
}
