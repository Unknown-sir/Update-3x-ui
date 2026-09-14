import { Input, InputNumber, Select } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function OpenvpnFields() {
  return (
    <>
      <FormField name={['settings', 'proto']} label="Protocol (proto)">
        <Select
          options={[
            { label: 'UDP', value: 'udp' },
            { label: 'TCP', value: 'tcp' },
          ]}
        />
      </FormField>
      <FormField name={['settings', 'subnet']} label="Subnet">
        <Input placeholder="10.8.0.0" />
      </FormField>
      <FormField name={['settings', 'netmask']} label="Netmask">
        <Input placeholder="255.255.255.0" />
      </FormField>
      <FormField name={['settings', 'speedLimitMbps']} label="Speed limit (Mbps, 0 = unlimited)">
        <InputNumber min={0} style={{ width: '100%' }} />
      </FormField>
    </>
  );
}
