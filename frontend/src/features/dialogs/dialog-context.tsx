import {
	createContext,
	useCallback,
	useContext,
	useRef,
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
	// The queue lives in a ref, with the state only mirroring it so consumers
	// re-render. `consumePendingMessage` has to return the message it removed,
	// and a `useState` updater cannot supply that: React only guarantees to run
	// it during the next render, so reading the value out of the updater returns
	// null exactly when the queue was just written to — which is the only case
	// this queue is ever used in. The ref is the source of truth instead.
	const pendingRef = useRef<PendingInitialMessage[]>([])
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
		pendingRef.current = [...pendingRef.current, msg]
		setPendingMessages(pendingRef.current)
	}, [])

	const consumePendingMessage = useCallback((dialogId: string) => {
		const queue = pendingRef.current
		const index = queue.findIndex((m) => m.dialogId === dialogId)
		if (index < 0) {
			return null
		}
		const consumed = queue[index]
		if (!consumed) {
			return null
		}
		pendingRef.current = [
			...queue.slice(0, index),
			...queue.slice(index + 1),
		]
		setPendingMessages(pendingRef.current)
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
