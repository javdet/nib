import { useState, type FormEvent } from 'react'
import nibLogo from '@/assets/nib-logo-transparent.png'
import { Button } from '@/components/ui/button'
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ApiError, extractErrorMessage } from '@/lib/api-client'
import { login } from '../api/auth'

type LoginPageProps = {
	onSuccess: () => Promise<void>
}

export function LoginPage({ onSuccess }: LoginPageProps) {
	const [token, setToken] = useState('')
	const [error, setError] = useState<string | null>(null)
	const [isSubmitting, setIsSubmitting] = useState(false)

	async function handleSubmit(e: FormEvent) {
		e.preventDefault()
		if (!token.trim()) return
		setIsSubmitting(true)
		setError(null)
		try {
			await login(token.trim())
			setToken('')
			await onSuccess()
		} catch (err) {
			setError(
				err instanceof ApiError && err.status === 401
					? 'The token was not accepted.'
					: extractErrorMessage(err),
			)
		} finally {
			setIsSubmitting(false)
		}
	}

	return (
		<div className="flex min-h-screen items-center justify-center bg-background p-4">
			<Card className="w-full max-w-sm">
				<CardHeader>
					<img src={nibLogo} alt="Nib" className="mb-2 h-8 w-auto self-start" />
					<CardTitle>Sign in</CardTitle>
					<CardDescription>
						Enter the API token this Nib server was started with
						(NIB_API_TOKEN).
					</CardDescription>
				</CardHeader>
				<CardContent>
					<form onSubmit={handleSubmit} className="flex flex-col gap-4">
						<div className="flex flex-col gap-2">
							<Label htmlFor="api-token">API token</Label>
							<Input
								id="api-token"
								type="password"
								autoComplete="current-password"
								autoFocus
								value={token}
								onChange={(e) => setToken(e.target.value)}
							/>
						</div>
						{error && <p className="text-sm text-destructive">{error}</p>}
						<Button type="submit" disabled={isSubmitting || !token.trim()}>
							{isSubmitting ? 'Signing in...' : 'Sign in'}
						</Button>
					</form>
				</CardContent>
			</Card>
		</div>
	)
}
