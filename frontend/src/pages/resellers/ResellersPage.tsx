import { useState } from 'react';
import {
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Space,
  Switch,
  Table,
  Tag,
  message,
} from 'antd';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  LinkOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';

import { usePageTitle } from '@/hooks/usePageTitle';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

const ResellerSchema = z
  .object({
    id: z.number(),
    username: z.string(),
    totalGB: z.number().nullable().optional(),
    expiryTime: z.number().nullable().optional(),
    speedLimitMbps: z.number().nullable().optional(),
    enable: z.boolean().nullable().optional(),
  })
  .loose();
const ResellerListSchema = z
  .array(ResellerSchema)
  .nullable()
  .transform((v) => v ?? []);
type Reseller = z.infer<typeof ResellerSchema>;

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } } as const;

export function resellerLoginLink(username: string): string {
  const raw = (window as unknown as { X_UI_BASE_PATH?: string }).X_UI_BASE_PATH || '/';
  const base = raw.endsWith('/') ? raw : `${raw}/`;
  return `${window.location.origin}${base}reseller/${encodeURIComponent(username)}`;
}

function copyText(value: string) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(value).then(
      () => message.success('Copied'),
      () => message.error(value),
    );
  } else {
    message.info(value);
  }
}

async function fetchResellers(): Promise<Reseller[]> {
  const msg = await HttpUtil.get('/panel/api/resellers/list', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to load resellers');
  return parseMsg(msg, ResellerListSchema, 'resellers/list').obj ?? [];
}

export default function ResellersPage() {
  usePageTitle();
  const queryClient = useQueryClient();
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<Reseller | null>(null);
  const [form] = Form.useForm();

  const listQuery = useQuery({ queryKey: ['resellers', 'list'], queryFn: fetchResellers });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['resellers'] });

  const saveMutation = useMutation({
    mutationFn: async (values: Record<string, unknown>) => {
      const body = JSON.stringify(values);
      const msg = editing
        ? await HttpUtil.post(`/panel/api/resellers/update/${editing.id}`, body, JSON_HEADERS)
        : await HttpUtil.post('/panel/api/resellers/add', body, JSON_HEADERS);
      if (!msg?.success) throw new Error(msg?.msg || 'Save failed');
      return msg;
    },
    onSuccess: (_, values) => {
      const username = editing ? editing.username : String(values.username || '');
      message.success('Saved');
      setModalOpen(false);
      setEditing(null);
      if (username) {
        const link = resellerLoginLink(username);
        Modal.success({
          title: `Reseller ${username} saved`,
          content: (
            <Space direction="vertical">
              <span>Dedicated login page:</span>
              <a href={link} target="_blank" rel="noreferrer">
                {link}
              </a>
              <Button icon={<CopyOutlined />} onClick={() => copyText(link)}>
                Copy link
              </Button>
            </Space>
          ),
        });
      }
      form.resetFields();
      invalidate();
    },
    onError: (e: Error) => message.error(e.message),
  });

  const delMutation = useMutation({
    mutationFn: async (id: number) => {
      const msg = await HttpUtil.post(`/panel/api/resellers/del/${id}`, undefined);
      if (!msg?.success) throw new Error(msg?.msg || 'Delete failed');
    },
    onSuccess: invalidate,
    onError: (e: Error) => message.error(e.message),
  });

  const openAdd = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ enable: true, totalGB: 0, expiryTime: 0, speedLimitMbps: 0 });
    setModalOpen(true);
  };

  const openEdit = (row: Reseller) => {
    setEditing(row);
    form.setFieldsValue({
      username: row.username,
      totalGB: Number(row.totalGB) || 0,
      expiryTime: Number(row.expiryTime) || 0,
      speedLimitMbps: Number(row.speedLimitMbps) || 0,
      enable: row.enable !== false,
    });
    setModalOpen(true);
  };

  return (
    <Card
      title="Resellers (0 = unlimited for volume / expiry / speed)"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>
          Add reseller
        </Button>
      }
    >
      <Table<Reseller>
        rowKey="id"
        loading={listQuery.isLoading}
        dataSource={listQuery.data ?? []}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 60 },
          { title: 'Username', dataIndex: 'username' },
          {
            title: 'Volume (GB)',
            render: (_, r) => (!r.totalGB ? <Tag>∞</Tag> : <Tag>{r.totalGB}</Tag>),
          },
          {
            title: 'Speed (Mbps)',
            render: (_, r) => (!r.speedLimitMbps ? <Tag>∞</Tag> : <Tag>{r.speedLimitMbps}</Tag>),
          },
          {
            title: 'Enabled',
            render: (_, r) =>
              r.enable === false ? <Tag color="red">Off</Tag> : <Tag color="green">On</Tag>,
          },
          {
            title: 'Login page',
            render: (_, r) => {
              const link = resellerLoginLink(r.username);
              return (
                <Space>
                  <a href={link} target="_blank" rel="noreferrer" title={link}>
                    <LinkOutlined /> open
                  </a>
                  <Button
                    size="small"
                    icon={<CopyOutlined />}
                    aria-label={`Copy login link for ${r.username}`}
                    onClick={() => copyText(link)}
                  />
                </Space>
              );
            },
          },
          {
            title: 'Actions',
            render: (_, r) => (
              <Space>
                <Button icon={<EditOutlined />} onClick={() => openEdit(r)} />
                <Button danger icon={<DeleteOutlined />} onClick={() => delMutation.mutate(r.id)} />
              </Space>
            ),
          },
        ]}
      />
      <Modal
        title={editing ? `Edit reseller ${editing.username}` : 'Add reseller'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMutation.isPending}
      >
        <Form form={form} layout="vertical" onFinish={(v) => saveMutation.mutate(v)}>
          {!editing && (
            <Form.Item name="username" label="Username" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
          )}
          {!editing && (
            <Form.Item name="password" label="Password" rules={[{ required: true }]}>
              <Input.Password />
            </Form.Item>
          )}
          {editing && (
            <Form.Item name="password" label="New password (empty = keep)">
              <Input.Password />
            </Form.Item>
          )}
          <Form.Item name="totalGB" label="Volume limit (GB, 0 = unlimited)">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="expiryTime" label="Expiry (unix ms, 0 = unlimited)">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="speedLimitMbps" label="Speed limit (Mbps, 0 = unlimited)">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="enable" label="Enabled" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
