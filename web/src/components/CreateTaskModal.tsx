import { useState } from 'react'
import { api } from '../api/client'
import { useToast } from './Toast'
import { Modal, Button, FormItem, Textarea, Select } from './index'

interface Props {
  open: boolean
  onClose: () => void
  onCreated: () => void
}

export default function CreateTaskModal({ open, onClose, onCreated }: Props) {
  const [goal, setGoal] = useState('')
  const [priority, setPriority] = useState(5)
  const [submitting, setSubmitting] = useState(false)
  const { showToast } = useToast()

  const handleSubmit = async () => {
    if (!goal.trim()) {
      showToast('请输入任务目标', 'warning')
      return
    }
    setSubmitting(true)
    try {
      await api.createTask({ goal: goal.trim(), priority })
      showToast('任务已创建', 'success')
      setGoal('')
      setPriority(5)
      onCreated()
      onClose()
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      open={open}
      title="创建任务"
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>取消</Button>
          <Button variant="primary" onClick={handleSubmit} loading={submitting}>
            {submitting ? '创建中...' : '创建任务'}
          </Button>
        </>
      }
    >
      <FormItem label="任务目标" required>
        <Textarea
          placeholder="例如：打开浏览器，搜索今天的 GitHub Trending，整理前十名"
          value={goal}
          onChange={e => setGoal(e.target.value)}
          rows={4}
          autoFocus
        />
      </FormItem>
      <FormItem label="优先级">
        <Select value={priority} onChange={e => setPriority(Number(e.target.value))}>
          <option value={0}>P0 - 最高</option>
          <option value={2}>P2</option>
          <option value={5}>P5 - 普通</option>
          <option value={8}>P8</option>
          <option value={9}>P9 - 最低</option>
        </Select>
      </FormItem>
    </Modal>
  )
}
