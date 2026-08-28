import { lazy, Suspense } from 'react'
import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { AppLayout } from '@/components/layout/app-layout'

const KnowledgePage = lazy(() =>
	import('@/features/knowledge/pages/knowledge-page').then((m) => ({
		default: m.KnowledgePage,
	})),
)
const PlansPage = lazy(() =>
	import('@/features/plans/pages/plans-page').then((m) => ({
		default: m.PlansPage,
	})),
)
const RulesPage = lazy(() =>
	import('@/features/rules/pages/rules-page').then((m) => ({
		default: m.RulesPage,
	})),
)
const ToolsPage = lazy(() =>
	import('@/features/tools/pages/tools-page').then((m) => ({
		default: m.ToolsPage,
	})),
)
const SkillsPage = lazy(() =>
	import('@/features/skills/pages/skills-page').then((m) => ({
		default: m.SkillsPage,
	})),
)
const VariablesPage = lazy(() =>
	import('@/features/variables/pages/variables-page').then((m) => ({
		default: m.VariablesPage,
	})),
)
const WorkplacePage = lazy(() =>
	import('@/features/workplace/pages/workplace-page').then((m) => ({
		default: m.WorkplacePage,
	})),
)
const WorkplaceDetail = lazy(() =>
	import('@/features/workplace/pages/workplace-detail').then((m) => ({
		default: m.WorkplaceDetail,
	})),
)
const IncidentsPage = lazy(() =>
	import('@/features/incidents/pages/incidents-page').then((m) => ({
		default: m.IncidentsPage,
	})),
)
const IncidentDetail = lazy(() =>
	import('@/features/incidents/pages/incident-detail').then((m) => ({
		default: m.IncidentDetail,
	})),
)
const SystemToolsPage = lazy(() =>
	import('@/features/system-tools/pages/system-tools-page').then((m) => ({
		default: m.SystemToolsPage,
	})),
)

function RedirectToTools() {
	const { search } = useLocation()
	return <Navigate to={`/tools${search}`} replace />
}

function PageFallback() {
	return (
		<div className="flex h-full items-center justify-center p-8 text-sm text-muted-foreground">
			Loading...
		</div>
	)
}

export function App() {
	return (
		<TooltipProvider delayDuration={400} skipDelayDuration={300}>
			<BrowserRouter>
				<Suspense fallback={<PageFallback />}>
					<Routes>
						<Route element={<AppLayout />}>
							<Route path="/" element={<Navigate to="/workplace" replace />} />
							<Route path="/workplace" element={<WorkplacePage />} />
							<Route path="/workplace/:id" element={<WorkplaceDetail />} />
							<Route path="/incidents" element={<IncidentsPage />} />
							<Route path="/incidents/:id" element={<IncidentDetail />} />
							<Route path="/projects" element={<Navigate to="/workplace" replace />} />
							<Route path="/projects/:id" element={<Navigate to="/workplace" replace />} />
							<Route path="/dialogs" element={<Navigate to="/workplace" replace />} />
							<Route path="/knowledge" element={<KnowledgePage />} />
							<Route path="/plans" element={<PlansPage />} />
							<Route path="/rules" element={<RulesPage />} />
							<Route path="/tools" element={<ToolsPage />} />
							<Route path="/skills" element={<SkillsPage />} />
							<Route path="/variables" element={<VariablesPage />} />
							<Route path="/system-tools" element={<SystemToolsPage />} />
							<Route path="/task-tracker" element={<RedirectToTools />} />
							<Route path="/mcp-connections" element={<RedirectToTools />} />
							<Route path="/base" element={<RedirectToTools />} />
						</Route>
					</Routes>
				</Suspense>
			</BrowserRouter>
		</TooltipProvider>
	)
}
