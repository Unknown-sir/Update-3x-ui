import { useRef, useState } from 'react';
import {
  Button,
  Card,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  message,
  type InputRef,
} from 'antd';
import { DeleteOutlined, EditOutlined, LogoutOutlined, PlusOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import dayjs, { type Dayjs } from 'dayjs';
function basePath(): string {
  const raw = (window as unknown as { X_UI_BASE_PATH?: string }).X_UI_BASE_PATH || '/';
  return raw.endsWith('/') ? raw : `${raw}/`;
}
function csrfToken(): string {
  return document.querySelector('meta[name="csrf-token"]')?.getAttribute('content') || '';
}
// Username embedded in dedicated login URLs (/reseller/<username>).
// Empty on the generic /reseller page, where it stays editable.
function lockedUsername(): string {
  const base = basePath();
  let rest = window.location.pathname;
  if (rest.startsWith(base)) rest = rest.slice(base.length);
  const segs = rest.split('/').filter(Boolean);
  if (segs.length >= 2 && segs[0] === 'reseller' && segs[1]) {
    try {
      return decodeURIComponent(segs[1]);
    } catch {
      return segs[1];
    }
  }
  return '';
}
async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${basePath()}panel/api/${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'X-Requested-With': 'XMLHttpRequest',
      'X-CSRF-Token': csrfToken(),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 404) {
    throw new Error('old-server');
  }
  if (res.status === 401 || res.status === 403) {
    throw new Error('auth');
  }
  const msg = (await res.json()) as { success: boolean; msg?: string; obj?: T };
  if (!msg?.success) throw new Error(msg?.msg || 'Request failed');
  return msg.obj as T;
}
interface PortalClient {
  email: string;
  totalGB?: number | null;
  expiryTime?: number | null;
  speedLimitMbps?: number | null;
  enable?: boolean | null;
}
interface PortalInbound {
  id: number;
  remark: string;
  protocol: string;
  port: number;
}
const GB = 1024 * 1024 * 1024;
export default function ResellerPortal() {
  const queryClient = useQueryClient();
  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<PortalClient | null>(null);
  const [loggedOut, setLoggedOut] = useState(false);
  const [form] = Form.useForm();
  const [loginForm] = Form.useForm();
  // Refs read the live DOM value at submit time. Browser autofill fills the
  // input without firing React change events, leaving the Form store empty
  // while text is visibly present — the ref bypasses that desync.
  const usernameRef = useRef<InputRef>(null);
  const passwordRef = useRef<InputRef>(null);
  const clientsQuery = useQuery({
    queryKey: ['reseller', 'clients'],
    queryFn: () => api<PortalClient[]>('GET', 'resellers/myClients'),
    retry: false,
    enabled: !loggedOut,
  });
  const inboundsQuery = useQuery({
    queryKey: ['reseller', 'inbounds'],
    queryFn: () => api<PortalInbound[]>('GET', 'resellers/myInbounds'),
    retry: false,
    enabled: !clientsQuery.isError,
  });
  const authed = !clientsQuery.isError;
  const serverOutdated =
    clientsQuery.error instanceof Error && clientsQuery.error.message === 'old-server';
  const clients = Array.isArray(clientsQuery.data) ? clientsQuery.data : [];
  const inbounds = Array.isArray(inboundsQuery.data) ? inboundsQuery.data : [];
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['reseller'] });
  };
  const dedicatedUser = lockedUsername();
  const loginMutation = useMutation({
    mutationFn: (values: { username?: string; password?: string }) => {
      const username = (
        dedicatedUser ||
        values.username ||
        usernameRef.current?.input?.value ||
        ''
      ).trim();
      const password = values.password || passwordRef.current?.input?.value || '';
      if (!username) throw new Error('Username is required');
      if (!password) throw new Error('Password is required');
      return api('POST', 'resellers/login', { username, password });
    },
    onSuccess: () => {
      loginForm.resetFields();
      setLoggedOut(false);
      invalidate();
      message.success('Logged in');
    },
    onError: (e: Error) => {
      if (e.message === 'old-server') {
        message.error('Panel is outdated — update it to v3.9.1 or newer first');
      } else if (e.message === 'auth') {
        message.error('Invalid username or password');
      } else {
        message.error(e.message);
      }
    },
  });
  const saveMutation = useMutation({
    mutationFn: async (values: {
      email: string;
      inboundIds?: number[];
      totalGB?: number;
      expiryDate?: Dayjs | null;
      speedLimitMbps?: number;
      enable?: boolean;
    }) => {
      const payload = {
        email: values.email.trim(),
        totalGB: Math.round(Number(values.totalGB || 0) * GB),
        expiryTime: values.expiryDate ? (values.expiryDate as Dayjs).valueOf() : 0,
        speedLimitMbps: Number(values.speedLimitMbps) || 0,
        enable: values.enable !== false,
        subId: '',
      };
      if (editing) {
        return api(
          'POST',
          `resellers/myClients/update/${encodeURIComponent(editing.email)}`,
          payload,
        );
      }
      if (!values.inboundIds?.length) throw new Error('Select at least one inbound');
      return api('POST', 'resellers/myClients/add', {
        client: payload,
        inboundIds: values.inboundIds,
      });
    },
    onSuccess: () => {
      message.success('Saved');
      setModalOpen(false);
      invalidate();
    },
    onError: (e: Error) => message.error(e.message),
  });
  const toggleMutation = useMutation({
    mutationFn: (v: { row: PortalClient; enable: boolean }) =>
      api('POST', 'resellers/myClients/setEnable', { email: v.row.email, enable: v.enable }),
    onSuccess: (_, v) => {
      message.success(v.enable ? 'Enabled' : 'Disabled');
      invalidate();
    },
    onError: (e: Error) => message.error(e.message),
  });
  const delMutation = useMutation({
    mutationFn: (row: PortalClient) =>
      api('POST', `resellers/myClients/del/${encodeURIComponent(row.email)}`),
    onSuccess: () => {
      message.success('Deleted');
      invalidate();
    },
    onError: (e: Error) => message.error(e.message),
  });
  const doLogout = async () => {
    try {
      await fetch(`${basePath()}logout`, {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken() },
      });
    } finally {
      queryClient.removeQueries({ queryKey: ['reseller'] });
      setLoggedOut(true);
    }
  };
  const openAdd = () => {
    setEditing(null);
    form.resetFields();
    form.setFieldsValue({ enable: true, totalGB: 0, speedLimitMbps: 0 });
    setModalOpen(true);
  };
  const openEdit = (row: PortalClient) => {
    setEditing(row);
    form.setFieldsValue({
      email: row.email,
      inboundIds: undefined,
      totalGB: Math.round(((Number(row.totalGB) || 0) / GB) * 100) / 100,
      expiryDate: row.expiryTime ? dayjs(Number(row.expiryTime)) : null,
      speedLimitMbps: Number(row.speedLimitMbps) || 0,
      enable: row.enable !== false,
    });
    setModalOpen(true);
  };
  if (clientsQuery.isPending) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 120 }}>
        {' '}
        <Spin size="large" />{' '}
      </div>
    );
  }
  if (!authed) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', paddingTop: 100 }}>
        {' '}
        <Card title="Reseller login" style={{ width: 360 }}>
          {serverOutdated && (
            <p style={{ color: '#ff4d4f' }}>
              Panel is outdated — update it to v3.9.1 or newer first.
            </p>
          )}{' '}
          <Form form={loginForm} layout="vertical" onFinish={(v) => loginMutation.mutate(v)}>
            {' '}
            {dedicatedUser ? (
              <Form.Item label="Username">
                <Tag style={{ fontSize: 15, padding: '4px 12px' }}>{dedicatedUser}</Tag>
              </Form.Item>
            ) : (
              <Form.Item name="username" label="Username">
                {' '}
                <Input ref={usernameRef} autoComplete="username" />{' '}
              </Form.Item>
            )}{' '}
            <Form.Item name="password" label="Password">
              {' '}
              <Input.Password ref={passwordRef} autoComplete="current-password" />{' '}
            </Form.Item>{' '}
            <Button type="primary" htmlType="submit" block loading={loginMutation.isPending}>
              {' '}
              Login{' '}
            </Button>{' '}
          </Form>{' '}
        </Card>{' '}
      </div>
    );
  }
  return (
    <div style={{ maxWidth: 1100, margin: '24px auto', padding: '0 16px' }}>
      {' '}
      <Card
        title="My clients"
        extra={
          <Space>
            {' '}
            <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>
              {' '}
              Add client{' '}
            </Button>{' '}
            <Button icon={<LogoutOutlined />} onClick={doLogout}>
              {' '}
              Logout{' '}
            </Button>{' '}
          </Space>
        }
      >
        {' '}
        <Table<PortalClient>
          rowKey="email"
          loading={clientsQuery.isFetching}
          dataSource={clients}
          pagination={{ pageSize: 20 }}
          columns={[
            { title: 'Email', dataIndex: 'email' },
            {
              title: 'Volume (GB)',
              render: (_, r) =>
                !r.totalGB ? <Tag>ظêئ</Tag> : <Tag>{(Number(r.totalGB) / GB).toFixed(1)}</Tag>,
            },
            {
              title: 'Expiry',
              render: (_, r) =>
                !r.expiryTime ? (
                  <Tag>ظêئ</Tag>
                ) : (
                  <Tag>{dayjs(Number(r.expiryTime)).format('YYYY-MM-DD')}</Tag>
                ),
            },
            {
              title: 'Speed (Mbps)',
              render: (_, r) =>
                !r.speedLimitMbps ? <Tag>ظêئ</Tag> : <Tag>{r.speedLimitMbps}</Tag>,
            },
            {
              title: 'Enabled',
              render: (_, r) => (
                <Switch
                  checked={r.enable !== false}
                  loading={toggleMutation.isPending}
                  onChange={(v) => toggleMutation.mutate({ row: r, enable: v })}
                />
              ),
            },
            {
              title: 'Actions',
              render: (_, r) => (
                <Space>
                  {' '}
                  <Button icon={<EditOutlined />} onClick={() => openEdit(r)} />{' '}
                  <Button
                    danger
                    icon={<DeleteOutlined />}
                    onClick={() =>
                      Modal.confirm({
                        title: `Delete ${r.email}?`,
                        onOk: () => delMutation.mutate(r),
                      })
                    }
                  />{' '}
                </Space>
              ),
            },
          ]}
        />{' '}
      </Card>{' '}
      <Modal
        title={editing ? `Edit ${editing.email}` : 'Add client'}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
        confirmLoading={saveMutation.isPending}
      >
        {' '}
        <Form form={form} layout="vertical" onFinish={(v) => saveMutation.mutate(v)}>
          {' '}
          {!editing && (
            <Form.Item name="email" label="Email" rules={[{ required: true }]}>
              {' '}
              <Input />{' '}
            </Form.Item>
          )}{' '}
          {!editing && (
            <Form.Item name="inboundIds" label="Inbounds" rules={[{ required: true }]}>
              {' '}
              <Select
                mode="multiple"
                options={inbounds.map((b) => ({
                  label: `${b.remark || b.protocol} :${b.port}`,
                  value: b.id,
                }))}
              />{' '}
            </Form.Item>
          )}{' '}
          <Form.Item name="totalGB" label="Volume (GB, 0 = unlimited)">
            {' '}
            <InputNumber min={0} style={{ width: '100%' }} />{' '}
          </Form.Item>{' '}
          <Form.Item name="expiryDate" label="Expiry (empty = unlimited)">
            {' '}
            <DatePicker style={{ width: '100%' }} />{' '}
          </Form.Item>{' '}
          <Form.Item name="speedLimitMbps" label="Speed (Mbps, 0 = reseller cap)">
            {' '}
            <InputNumber min={0} style={{ width: '100%' }} />{' '}
          </Form.Item>{' '}
          <Form.Item name="enable" label="Enabled" valuePropName="checked">
            {' '}
            <Switch />{' '}
          </Form.Item>{' '}
        </Form>{' '}
      </Modal>{' '}
    </div>
  );
}
