import * as React from 'react'

import { cn } from '@/lib/utils'
import { fieldClasses } from '@/components/ui/input'

const Textarea = React.forwardRef<
	HTMLTextAreaElement,
	React.TextareaHTMLAttributes<HTMLTextAreaElement>
>(function Textarea({ className, ...props }, ref) {
	return (
		<textarea
			className={cn(fieldClasses, 'flex resize-y px-3 py-2 text-sm', className)}
			ref={ref}
			{...props}
		/>
	)
})

export { Textarea }
