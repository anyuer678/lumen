import type { ReactNode, ButtonHTMLAttributes, InputHTMLAttributes, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react'

// ---- Button ----
interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'default' | 'primary' | 'success' | 'danger' | 'warning' | 'ghost' | 'link'
  size?: 'small' | 'default' | 'large'
  block?: boolean
  loading?: boolean
}

export function Button({ variant = 'default', size, block, loading, children, className = '', ...props }: ButtonProps) {
  const classes = [
    'kb-btn',
    variant !== 'default' && `kb-btn--${variant}`,
    size && `kb-btn--${size}`,
    block && 'kb-btn--block',
    className
  ].filter(Boolean).join(' ')

  return (
    <button className={classes} disabled={loading || props.disabled} {...props}>
      {loading && <span className="kb-loading" />}
      {children}
    </button>
  )
}

// ---- Card ----
interface CardProps {
  title?: string
  extra?: ReactNode
  shadow?: boolean | 'hover'
  children: ReactNode
  className?: string
  style?: React.CSSProperties
  footer?: ReactNode
}

export function Card({ title, extra, shadow, children, className = '', style, footer }: CardProps) {
  const classes = [
    'kb-card',
    shadow === true && 'kb-card--shadow',
    shadow === 'hover' && 'kb-card--shadow-hover',
    className
  ].filter(Boolean).join(' ')

  return (
    <div className={classes} style={style}>
      {(title || extra) && (
        <div className="kb-card__header">
          {title && <div className="kb-card__title">{title}</div>}
          {extra}
        </div>
      )}
      <div className="kb-card__body">{children}</div>
      {footer && <div className="kb-card__footer">{footer}</div>}
    </div>
  )
}

// ---- Badge ----
interface BadgeProps {
  variant?: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  children: ReactNode
  className?: string
}

export function Badge({ variant = 'info', children, className = '' }: BadgeProps) {
  return (
    <span className={`kb-badge kb-badge--${variant} ${className}`.trim()}>
      {children}
    </span>
  )
}

// ---- Progress ----
interface ProgressProps {
  value: number
  variant?: 'default' | 'success' | 'warning' | 'danger'
  showText?: boolean
  className?: string
}

export function Progress({ value, variant = 'default', showText = true, className = '' }: ProgressProps) {
  const clampedValue = Math.max(0, Math.min(100, value))
  return (
    <div className={`kb-progress ${className}`}>
      <div className="kb-progress__track">
        <div
          className={`kb-progress__bar ${variant !== 'default' ? `kb-progress__bar--${variant}` : ''}`}
          style={{ width: `${clampedValue}%` }}
        />
      </div>
      {showText && <span className="kb-progress__text">{Math.round(clampedValue)}%</span>}
    </div>
  )
}

// ---- Input ----
interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  error?: boolean
}

export function Input({ error, className = '', ...props }: InputProps) {
  return (
    <input
      className={`kb-input ${error ? 'kb-input--error' : ''} ${className}`.trim()}
      {...props}
    />
  )
}

// ---- Select ----
interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {}

export function Select({ className = '', children, ...props }: SelectProps) {
  return (
    <select className={`kb-select ${className}`} {...props}>
      {children}
    </select>
  )
}

// ---- Textarea ----
interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {}

export function Textarea({ className = '', ...props }: TextareaProps) {
  return <textarea className={`kb-textarea ${className}`} {...props} />
}

// ---- FormItem ----
interface FormItemProps {
  label: string
  required?: boolean
  hint?: string
  error?: string
  children: ReactNode
}

export function FormItem({ label, required, hint, error, children }: FormItemProps) {
  return (
    <div className="kb-form-item">
      <label className={`kb-form-label ${required ? 'kb-form-label--required' : ''}`}>
        {label}
      </label>
      {children}
      {hint && !error && <div className="kb-form-hint">{hint}</div>}
      {error && <div className="kb-form-error">{error}</div>}
    </div>
  )
}

// ---- Modal ----
interface ModalProps {
  open: boolean
  title?: string
  onClose: () => void
  footer?: ReactNode
  children: ReactNode
  width?: number
}

export function Modal({ open, title, onClose, footer, children, width }: ModalProps) {
  if (!open) return null

  return (
    <div className="kb-modal-mask" onClick={onClose}>
      <div
        className="kb-modal"
        style={width ? { width } : undefined}
        onClick={e => e.stopPropagation()}
      >
        {title && (
          <div className="kb-modal__header">
            <div className="kb-modal__title">{title}</div>
            <button className="kb-modal__close" onClick={onClose}>×</button>
          </div>
        )}
        <div className="kb-modal__body">{children}</div>
        {footer && <div className="kb-modal__footer">{footer}</div>}
      </div>
    </div>
  )
}

// ---- Loading ----
interface LoadingProps {
  block?: boolean
  text?: string
}

export function Loading({ block, text }: LoadingProps) {
  if (block) {
    return (
      <div className="kb-loading--block">
        <div className="kb-loading" />
        {text && <span style={{ marginLeft: 8, color: 'var(--color-text-3)' }}>{text}</span>}
      </div>
    )
  }
  return <span className="kb-loading" />
}

// ---- Empty ----
interface EmptyProps {
  icon?: string
  text?: string
  children?: ReactNode
}

export function Empty({ icon = '📭', text = '暂无数据', children }: EmptyProps) {
  return (
    <div className="kb-empty">
      <div className="kb-empty__icon">{icon}</div>
      <div className="kb-empty__text">{text}</div>
      {children}
    </div>
  )
}

// ---- Statistic ----
interface StatisticProps {
  label: string
  value: string | number
  suffix?: string
  className?: string
}

export function Statistic({ label, value, suffix, className = '' }: StatisticProps) {
  return (
    <div className={`kb-statistic ${className}`}>
      <div className="kb-statistic__label">{label}</div>
      <div className="kb-statistic__value">
        {value}
        {suffix && <span className="kb-statistic__suffix">{suffix}</span>}
      </div>
    </div>
  )
}

// ---- Timeline ----
interface TimelineItemProps {
  dot?: 'success' | 'primary' | 'danger' | 'default'
  children: ReactNode
}

export function TimelineItem({ dot = 'default', children }: TimelineItemProps) {
  return (
    <div className="kb-timeline__item">
      <div className={`kb-timeline__dot kb-timeline__dot--${dot}`} />
      <div className="kb-timeline__content">{children}</div>
    </div>
  )
}

export function Timeline({ children }: { children: ReactNode }) {
  return <div className="kb-timeline">{children}</div>
}

// ---- Tabs ----
interface TabsProps {
  items: { key: string; label: string }[]
  active: string
  onChange: (key: string) => void
  className?: string
}

export function Tabs({ items, active, onChange, className = '' }: TabsProps) {
  return (
    <div className={`kb-tabs ${className}`}>
      {items.map(item => (
        <button
          key={item.key}
          className={`kb-tabs__item ${active === item.key ? 'kb-tabs__item--active' : ''}`}
          onClick={() => onChange(item.key)}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}

// ---- Descriptions ----
interface DescriptionsItemProps {
  label: string
  children: ReactNode
}

export function DescriptionsItem({ label, children }: DescriptionsItemProps) {
  return (
    <div className="kb-descriptions__item">
      <div className="kb-descriptions__label">{label}</div>
      <div className="kb-descriptions__content">{children}</div>
    </div>
  )
}

export function Descriptions({ children }: { children: ReactNode }) {
  return <div className="kb-descriptions">{children}</div>
}
