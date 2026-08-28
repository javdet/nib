import {
	createContext,
	useCallback,
	useContext,
	useState,
	type ReactNode,
} from 'react'

export interface PendingInitialMessage {
	dialogId: string
	text: string
}

interface DialogContextValue {
	activeDialogId: string | null
	setActiveDialogId: (id: string | null) => void
	dialogsVersion: number
	bumpDialogsVersion: () => void
	actionPlanVersion: number
	bumpActionPlanVersion: () => void
	pendingMessages: PendingInitialMessage[]
	enqueuePendingMessage: (msg: PendingInitialMessage) => void
	consumePendingMessage: (dialogId: string) => PendingInitialMessage | null
}

const DialogContext = createContext<DialogContextValue | null>(null)

export function DialogProvider({ children }: { children: ReactNode }) {
	const [activeDialogId, setActiveDialogId] = useState<string | null>(null)
	const [dialogsVersion, setDialogsVersion] = useState(0)
	const [actionPlanVersion, setActionPlanVersion] = useState(0)
	const [pendingMessages, setPendingMessages] = useState<
		PendingInitialMessage[]
	>([])

	const bumpDialogsVersion = useCallback(() => {
		setDialogsVersion((v) => v + 1)
	}, [])

	const bumpActionPlanVersion = useCallback(() => {
		setActionPlanVersion((v) => v + 1)
	}, [])

	const enqueuePendingMessage = useCallback((msg: PendingInitialMessage) => {
		setPendingMessages((prev) => [...prev, msg])
	}, [])

	const consumePendingMessage = useCallback((dialogId: string) => {
		let consumed: PendingInitialMessage | null = null
		setPendingMessages((prev) => {
			const index = prev.findIndex((m) => m.dialogId === dialogId)
			if (index < 0) {
				return prev
			}
			const next = prev[index]
			if (!next) {
				return prev
			}
			consumed = next
			return [...prev.slice(0, index), ...prev.slice(index + 1)]
		})
		return consumed
	}, [])

	return (
		<DialogContext
			value={{
				activeDialogId,
				setActiveDialogId,
				dialogsVersion,
				bumpDialogsVersion,
				actionPlanVersion,
				bumpActionPlanVersion,
				pendingMessages,
				enqueuePendingMessage,
				consumePendingMessage,
			}}
		>
			{children}
		</DialogContext>
	)
}

export function useDialog() {
	const ctx = useContext(DialogContext)
	if (!ctx) {
		throw new Error('useDialog must be used within DialogProvider')
	}
	return ctx
}
