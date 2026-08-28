import { Outlet } from 'react-router'
import { SidebarProvider } from './sidebar-context'
import { Sidebar } from './sidebar'
import { Header } from './header'
import { MainSplitLayout } from './main-split-layout'
import { DialogProvider } from '@/features/dialogs/dialog-context'
import { ModeProvider } from '@/features/modes/mode-context'
import { ProjectProvider } from '@/features/projects/project-context'
import { EnvironmentProvider } from '@/features/projects/environment-context'
import { CloudProvider } from '@/features/projects/cloud-context'
import { LocationProvider } from '@/features/projects/location-context'
import { SelectionSync } from '@/features/projects/selection-sync'

export function AppLayout() {
	return (
		<ModeProvider>
			<DialogProvider>
				<ProjectProvider>
					<EnvironmentProvider>
						<CloudProvider>
							<LocationProvider>
								<SelectionSync />
								<SidebarProvider>
									<div className="app-shell flex h-screen overflow-hidden">
										<Sidebar />
										<div className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden">
											<MainSplitLayout>
												<Outlet />
											</MainSplitLayout>
											<Header />
										</div>
									</div>
								</SidebarProvider>
							</LocationProvider>
						</CloudProvider>
					</EnvironmentProvider>
				</ProjectProvider>
			</DialogProvider>
		</ModeProvider>
	)
}
