import { Input, InputNumber, Select } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function CiscoFields() {
  return (
    <>
      <FormField name={['settings', 'auth']} label="Auth mode">
        <Select
          options={[
            { label: 'plain', value: 'plain' },
            { label: 'certificate', value: 'certificate' },
          ]}
        />
      </FormField>
      <FormField name={['settings', 'subnet']} label="IPv4 network">
        <Input placeholder="10.9.0.0/24" />
      </FormField>
      <FormField name={['settings', 'speedLimitMbps']} label="Speed limit (Mbps, 0 = unlimited)">
        <InputNumber min={0} style={{ width: '100%' }} />
      </FormField>
    </>
  );
}
