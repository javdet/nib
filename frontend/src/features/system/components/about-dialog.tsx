import { useEffect, useState } from 'react'
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { getVersion } from '@/features/system/api/version'

const REPOSITORY_URL = 'https://github.com/javdet/nib'
const LICENSE_URL = 'https://github.com/javdet/nib/blob/main/LICENSE'
const LICENSE_LABEL = 'Nib Non-Commercial License 1.0'

type AboutDialogProps = {
	open: boolean
	onOpenChange: (open: boolean) => void
}

export function AboutDialog({ open, onOpenChange }: AboutDialogProps) {
	const [version, setVersion] = useState<string>('…')

	useEffect(() => {
		if (!open) {
			return
		}

		const controller = new AbortController()

		getVersion()
			.then((info) => {
				if (!controller.signal.aborted) {
					setVersion(info.version)
				}
			})
			.catch(() => {
				if (!controller.signal.aborted) {
					setVersion('unknown')
				}
			})

		return () => controller.abort()
	}, [open])

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>About Nib</DialogTitle>
				</DialogHeader>

				<dl className="grid grid-cols-[130px_1fr] gap-x-4 gap-y-3 text-sm">
					<dt className="text-muted-foreground">Nib Version</dt>
					<dd>{version}</dd>

					<dt className="text-muted-foreground">Source Code</dt>
					<dd>
						<a
							href={REPOSITORY_URL}
							target="_blank"
							rel="noopener noreferrer"
							className="text-primary hover:underline"
						>
							{REPOSITORY_URL}
						</a>
					</dd>

					<dt className="text-muted-foreground">License</dt>
					<dd>
						<a
							href={LICENSE_URL}
							target="_blank"
							rel="noopener noreferrer"
							className="text-primary hover:underline"
						>
							{LICENSE_LABEL}
						</a>
					</dd>
				</dl>

				<DialogFooter className="sm:justify-start">
					<DialogClose asChild>
						<Button type="button">Close</Button>
					</DialogClose>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
