import { useState, useEffect, useCallback } from 'react'
import { Braces, KeyRound, Pencil, Plus, Trash2 } from 'lucide-react'
import { IconButton } from '@/components/ui/icon-button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { VariableDialog } from '../components/variable-dialog'
import { SecretDialog } from '@/features/secrets/components/secret-dialog'
import {
	listVariables,
	createVariable,
	updateVariable,
	deleteVariable,
	type Variable,
	type VariableInput,
} from '../api/variables'
import {
	listSecrets,
	createSecret,
	updateSecret,
	deleteSecret,
	type Secret,
	type SecretInput,
} from '@/features/secrets/api/secrets'
import { cn } from '@/lib/utils'

function formatScopeLabel(scope: string, scopeName?: string): string {
	if (scopeName?.trim()) {
		return `${scope} · ${scopeName.trim()}`
	}
	return scope
}

export function VariablesPage() {
	const [variables, setVariables] = useState<Variable[]>([])
	const [variablesLoading, setVariablesLoading] = useState(true)
	const [variableDialogOpen, setVariableDialogOpen] = useState(false)
	const [editingVariable, setEditingVariable] = useState<Variable | null>(null)

	const [secrets, setSecrets] = useState<Secret[]>([])
	const [secretsLoading, setSecretsLoading] = useState(true)
	const [secretDialogOpen, setSecretDialogOpen] = useState(false)
	const [editingSecret, setEditingSecret] = useState<Secret | null>(null)

	const [error, setError] = useState<string | null>(null)

	useEffect(() => {
		listVariables()
			.then(setVariables)
			.catch(() => setVariables([]))
			.finally(() => setVariablesLoading(false))
	}, [])

	useEffect(() => {
		listSecrets()
			.then(setSecrets)
			.catch(() => setSecrets([]))
			.finally(() => setSecretsLoading(false))
	}, [])

	const handleSaveVariable = useCallback(
		async (input: VariableInput) => {
			setError(null)
			try {
				if (editingVariable) {
					const updated = await updateVariable(editingVariable.id, input)
					setVariables((prev) =>
						prev.map((v) => (v.id === editingVariable.id ? updated : v)),
					)
				} else {
					const created = await createVariable(input)
					setVariables((prev) => [...prev, created])
				}
				setVariableDialogOpen(false)
				setEditingVariable(null)
			} catch (err) {
				setError(
					err instanceof Error ? err.message : 'Failed to save variable',
				)
			}
		},
		[editingVariable],
	)

	const handleDeleteVariable = useCallback(async (id: string) => {
		setError(null)
		try {
			await deleteVariable(id)
			setVariables((prev) => prev.filter((v) => v.id !== id))
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to delete variable',
			)
		}
	}, [])

	const handleSaveSecret = useCallback(
		async (input: SecretInput) => {
			setError(null)
			try {
				if (editingSecret) {
					const updated = await updateSecret(editingSecret.id, input)
					setSecrets((prev) =>
						prev.map((s) => (s.id === editingSecret.id ? updated : s)),
					)
				} else {
					const created = await createSecret(input)
					setSecrets((prev) => [...prev, created])
				}
				setSecretDialogOpen(false)
				setEditingSecret(null)
			} catch (err) {
				setError(
					err instanceof Error ? err.message : 'Failed to save secret',
				)
			}
		},
		[editingSecret],
	)

	const handleDeleteSecret = useCallback(async (id: string) => {
		setError(null)
		try {
			await deleteSecret(id)
			setSecrets((prev) => prev.filter((s) => s.id !== id))
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to delete secret',
			)
		}
	}, [])

	return (
		<div className="space-y-6">
			<h2 className="text-2xl font-bold tracking-tight">Variables</h2>
			<p className="text-sm text-muted-foreground">
				Manage template variables and encrypted secrets. Reference variables as{' '}
				<code className="rounded bg-muted px-1 py-0.5 text-xs">
					{'{{ .scope.Name }}'}
				</code>
				.
			</p>

			{error && (
				<div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
					{error}
				</div>
			)}

			<Tabs defaultValue="variables">
				<TabsList>
					<TabsTrigger value="variables">Variables</TabsTrigger>
					<TabsTrigger value="secrets">Secrets</TabsTrigger>
				</TabsList>

				<TabsContent value="variables">
					<Card>
						<CardHeader>
							<div className="flex items-center justify-between">
								<CardTitle>Variables</CardTitle>
								<IconButton
									size="sm"
									tooltip="Add variable"
									onClick={() => {
										setEditingVariable(null)
										setVariableDialogOpen(true)
									}}
								>
									<Plus className="h-4 w-4" />
								</IconButton>
							</div>
						</CardHeader>
						<CardContent>
							{variablesLoading ? (
								<p className="py-4 text-center text-sm text-muted-foreground">
									Loading...
								</p>
							) : variables.length === 0 ? (
								<div className="flex flex-col items-center gap-2 py-4 text-muted-foreground">
									<Braces className="h-8 w-8" />
									<p className="text-sm">No variables added yet.</p>
								</div>
							) : (
								<div className="space-y-3">
									{variables.map((variable) => (
										<div
											key={variable.id}
											className={cn(
												'flex items-start justify-between rounded-lg border p-3 transition-colors hover:bg-muted/50',
											)}
										>
											<div className="min-w-0 flex-1">
												<div className="flex items-center gap-2">
													<Braces className="h-4 w-4 shrink-0 text-muted-foreground" />
													<span className="text-base font-semibold">
														{variable.name}
													</span>
													{variable.kind === 'list' && (
														<Badge variant="secondary" className="text-xs">
															list
														</Badge>
													)}
												</div>
												<p className="mt-0.5 pl-6 text-xs text-muted-foreground">
													{formatScopeLabel(
														variable.scope,
														variable.scopeName,
													)}
												</p>
											</div>
											<div className="ml-2 flex shrink-0 gap-1">
												<IconButton
													variant="ghost"
													size="sm"
													tooltip="Edit variable"
													onClick={() => {
														setEditingVariable(variable)
														setVariableDialogOpen(true)
													}}
												>
													<Pencil className="h-4 w-4" />
												</IconButton>
												<IconButton
													variant="ghost"
													size="sm"
													disabled={!variable.deletable}
													tooltip={
														variable.deletable
															? 'Delete variable'
															: 'Built-in variables cannot be deleted'
													}
													onClick={() => handleDeleteVariable(variable.id)}
												>
													<Trash2 className="h-4 w-4" />
												</IconButton>
											</div>
										</div>
									))}
								</div>
							)}
						</CardContent>
					</Card>
				</TabsContent>

				<TabsContent value="secrets">
					<Card>
						<CardHeader>
							<div className="flex items-center justify-between">
								<CardTitle>Secrets</CardTitle>
								<IconButton
									size="sm"
									tooltip="Add secret"
									onClick={() => {
										setEditingSecret(null)
										setSecretDialogOpen(true)
									}}
								>
									<Plus className="h-4 w-4" />
								</IconButton>
							</div>
						</CardHeader>
						<CardContent>
							{secretsLoading ? (
								<p className="py-4 text-center text-sm text-muted-foreground">
									Loading...
								</p>
							) : secrets.length === 0 ? (
								<div className="flex flex-col items-center gap-2 py-4 text-muted-foreground">
									<KeyRound className="h-8 w-8" />
									<p className="text-sm">No secrets added yet.</p>
								</div>
							) : (
								<div className="space-y-3">
									{secrets.map((secret) => (
										<div
											key={secret.id}
											className={cn(
												'flex items-start justify-between rounded-lg border p-3 transition-colors hover:bg-muted/50',
											)}
										>
											<div className="min-w-0 flex-1">
												<div className="flex items-center gap-2">
													<KeyRound className="h-4 w-4 shrink-0 text-muted-foreground" />
													<span className="text-base font-semibold">
														{secret.name}
													</span>
												</div>
												<p className="mt-0.5 pl-6 text-xs text-muted-foreground">
													{formatScopeLabel(
														secret.scope,
														secret.scopeName,
													)}
												</p>
											</div>
											<div className="ml-2 flex shrink-0 gap-1">
												<IconButton
													variant="ghost"
													size="sm"
													tooltip="Edit secret"
													onClick={() => {
														setEditingSecret(secret)
														setSecretDialogOpen(true)
													}}
												>
													<Pencil className="h-4 w-4" />
												</IconButton>
												<IconButton
													variant="ghost"
													size="sm"
													tooltip="Delete secret"
													onClick={() => handleDeleteSecret(secret.id)}
												>
													<Trash2 className="h-4 w-4" />
												</IconButton>
											</div>
										</div>
									))}
								</div>
							)}
						</CardContent>
					</Card>
				</TabsContent>
			</Tabs>

			<VariableDialog
				open={variableDialogOpen}
				onOpenChange={(open) => {
					setVariableDialogOpen(open)
					if (!open) setEditingVariable(null)
				}}
				onSave={handleSaveVariable}
				initialVariable={editingVariable}
			/>

			<SecretDialog
				open={secretDialogOpen}
				onOpenChange={(open) => {
					setSecretDialogOpen(open)
					if (!open) setEditingSecret(null)
				}}
				onSave={handleSaveSecret}
				initialSecret={editingSecret}
			/>
		</div>
	)
}
