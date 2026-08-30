import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Save } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
	Form,
	FormField,
	FormItem,
	FormLabel,
	FormControl,
	FormMessage,
} from '@/components/ui/form'
import { extractErrorMessage } from '@/lib/api-client'
import {
	type CompanyInfo,
	getCompany,
	updateCompany,
} from '../api/company'

const schema = z.object({
	companyName: z.string(),
	companyDescription: z.string(),
	versionControlSystem: z.string(),
	gitBaseUrl: z.string(),
	gitUsername: z.string(),
	gitEmail: z.union([z.literal(''), z.string().email()]),
	ciCdSystem: z.string(),
	taskTracker: z.string(),
	issueProject: z.string(),
	wiki: z.string(),
	messenger: z.string(),
})

type FormValues = z.infer<typeof schema>
type SaveStatus = 'idle' | 'saved' | 'failed'

const emptyValues: FormValues = {
	companyName: '',
	companyDescription: '',
	versionControlSystem: '',
	gitBaseUrl: '',
	gitUsername: '',
	gitEmail: '',
	ciCdSystem: '',
	taskTracker: '',
	issueProject: '',
	wiki: '',
	messenger: '',
}

function toFormValues(info: Partial<CompanyInfo>): FormValues {
	return {
		companyName: info.companyName ?? '',
		companyDescription: info.companyDescription ?? '',
		versionControlSystem: info.versionControlSystem ?? '',
		gitBaseUrl: info.gitBaseUrl ?? '',
		gitUsername: info.gitUsername ?? '',
		gitEmail: info.gitEmail ?? '',
		ciCdSystem: info.ciCdSystem ?? '',
		taskTracker: info.taskTracker ?? '',
		issueProject: info.issueProject ?? '',
		wiki: info.wiki ?? '',
		messenger: info.messenger ?? '',
	}
}

function normalizeFormValues(values: FormValues): FormValues {
	return {
		...values,
		ciCdSystem: values.ciCdSystem.trim() || values.versionControlSystem,
	}
}

function formValuesEqual(a: FormValues, b: FormValues): boolean {
	const left = normalizeFormValues(a)
	const right = normalizeFormValues(b)
	return (Object.keys(left) as (keyof FormValues)[]).every(
		(key) => left[key] === right[key],
	)
}

export function CompanyInfoCard() {
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const [saveStatus, setSaveStatus] = useState<SaveStatus>('idle')
	const [savedValues, setSavedValues] = useState<FormValues | null>(null)

	const form = useForm<FormValues>({
		resolver: zodResolver(schema),
		defaultValues: emptyValues,
	})
	const watchedValues = form.watch()
	const dirty = useMemo(
		() => savedValues !== null && !formValuesEqual(watchedValues, savedValues),
		[savedValues, watchedValues],
	)

	useEffect(() => {
		async function load() {
			setLoading(true)
			setError(null)
			try {
				const info = await getCompany()
				const values = toFormValues(info)
				form.reset(values)
				setSavedValues(values)
			} catch (err) {
				setError(extractErrorMessage(err))
			} finally {
				setLoading(false)
			}
		}
		load()
	}, [form])

	async function handleSubmit(values: FormValues) {
		setSaving(true)
		setError(null)
		setSaveStatus('idle')
		try {
			const payload: CompanyInfo = {
				...values,
				ciCdSystem: values.ciCdSystem.trim() || values.versionControlSystem,
			}
			const saved = await updateCompany(payload)
			const nextValues = toFormValues(saved)
			form.reset(nextValues)
			setSavedValues(nextValues)
			setSaveStatus('saved')
		} catch (err) {
			setError(extractErrorMessage(err))
			setSaveStatus('failed')
		} finally {
			setSaving(false)
		}
	}

	if (loading) {
		return (
			<Card>
				<CardHeader>
					<CardTitle>Company Information</CardTitle>
				</CardHeader>
				<CardContent>
					<p className="text-sm text-muted-foreground">Loading...</p>
				</CardContent>
			</Card>
		)
	}

	return (
		<Card>
			<CardHeader>
				<CardTitle>Company Information</CardTitle>
			</CardHeader>
			<CardContent className="space-y-4">
				{error && (
					<div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
						{error}
					</div>
				)}

				<Form {...form}>
					<form
						onSubmit={form.handleSubmit(handleSubmit)}
						className="space-y-4"
					>
						<FormField
							control={form.control}
							name="companyName"
							render={({ field }) => (
								<FormItem>
									<FormLabel>Company name</FormLabel>
									<FormControl>
										<Input placeholder="Acme Corp" {...field} />
									</FormControl>
									<FormMessage />
								</FormItem>
							)}
						/>

						<FormField
							control={form.control}
							name="companyDescription"
							render={({ field }) => (
								<FormItem>
									<FormLabel>Company description</FormLabel>
									<FormControl>
										<textarea
											className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
											placeholder="Brief description of the company"
											{...field}
										/>
									</FormControl>
									<FormMessage />
								</FormItem>
							)}
						/>

						<div className="grid gap-4 sm:grid-cols-2">
							<FormField
								control={form.control}
								name="versionControlSystem"
								render={({ field }) => (
									<FormItem>
										<FormLabel>Version control system</FormLabel>
										<FormControl>
											<Input placeholder="GitHub" {...field} />
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="gitBaseUrl"
								render={({ field }) => (
									<FormItem>
										<FormLabel>GIT base URL</FormLabel>
										<FormControl>
											<Input
												placeholder="https://github.com/my-org"
												{...field}
											/>
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="gitUsername"
								render={({ field }) => (
									<FormItem>
										<FormLabel>Git username</FormLabel>
										<FormControl>
											<Input placeholder="agent-runner" {...field} />
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="gitEmail"
								render={({ field }) => (
									<FormItem>
										<FormLabel>Git email</FormLabel>
										<FormControl>
											<Input
												type="email"
												placeholder="agent-runner@localhost"
												{...field}
											/>
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="ciCdSystem"
								render={({ field }) => (
									<FormItem>
										<FormLabel>CI/CD system</FormLabel>
										<FormControl>
											<Input
												placeholder="Same as version control system"
												{...field}
											/>
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="taskTracker"
								render={({ field }) => (
									<FormItem>
										<FormLabel>Task tracker</FormLabel>
										<FormControl>
											<Input placeholder="Jira" {...field} />
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="issueProject"
								render={({ field }) => (
									<FormItem>
										<FormLabel>Issue project</FormLabel>
										<FormControl>
											<Input placeholder="DEVOPS" {...field} />
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="wiki"
								render={({ field }) => (
									<FormItem>
										<FormLabel>Wiki</FormLabel>
										<FormControl>
											<Input placeholder="Confluence" {...field} />
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>

							<FormField
								control={form.control}
								name="messenger"
								render={({ field }) => (
									<FormItem className="sm:col-span-2">
										<FormLabel>Command messenger</FormLabel>
										<FormControl>
											<Input placeholder="Slack" {...field} />
										</FormControl>
										<FormMessage />
									</FormItem>
								)}
							/>
						</div>

						<div className="flex items-center justify-end gap-3">
							{saveStatus === 'saved' && !dirty && (
								<span
									className="text-sm text-green-600"
									aria-live="polite"
								>
									Saved
								</span>
							)}
							{saveStatus === 'failed' && (
								<span
									className="text-sm text-destructive"
									aria-live="polite"
								>
									Save failed
								</span>
							)}
							<Button type="submit" size="sm" disabled={saving || !dirty}>
								<Save className="h-4 w-4" />
								{saving ? 'Saving…' : 'Save'}
							</Button>
						</div>
					</form>
				</Form>
			</CardContent>
		</Card>
	)
}
