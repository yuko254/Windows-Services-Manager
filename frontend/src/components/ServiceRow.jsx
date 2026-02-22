import { memo, useCallback } from 'react';
import { TableRow, TableCell, Text, Switch, Tooltip, Button } from '@fluentui/react-components';
import { Play24Regular, Stop24Regular, Delete24Regular, Desktop24Regular } from '@fluentui/react-icons';

const ServiceRow = memo(({ service, onStart, onStop, onDelete, onMonitor, onAutoStartToggle }) => {
  const handleStart = useCallback(() => onStart(service.id), [service.id, onStart]);
  const handleStop = useCallback(() => onStop(service.id), [service.id, onStop]);
  const handleDelete = useCallback(() => onDelete(service.id), [service.id, onDelete]);
  const handleMonitor = useCallback(() => onMonitor(service.id,service.status), [service.id, service.status, onMonitor]);
  const handleAutoStartToggle = useCallback((checked) => onAutoStartToggle(service.id, checked), [service.id, onAutoStartToggle]);

  return (
    <TableRow key={service.id} className="win11-table-row">
      <TableCell>
        <Text weight="semibold" size="300">{service.name}</Text>
        <br />
        <Text size="200" style={{ color: '#666' }}>
          PID: {service.pid || 'N/A'}
        </Text>
      </TableCell>
      <TableCell>
        <div className={`service-status ${service.status}`}>
          <div style={{
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            backgroundColor: service.status === 'running' ? '#107c10' : 
                           service.status === 'error' ? '#c42b1c' : '#605e5c'
          }}></div>
          {service.status === 'running' ? 'Running' : 
           service.status === 'error' ? 'Error' : 'Stopped'}
        </div>
      </TableCell>
      <TableCell>
        <Text size="200" style={{ wordBreak: 'break-all' }}>
          {service.exePath}
        </Text>
        {service.args && (
          <>
            <br />
            <Text size="100" style={{ color: '#666', fontStyle: 'italic' }}>
              Args: {service.args}
            </Text>
          </>
        )}
      </TableCell>
      <TableCell>
        <Switch
          checked={service.autoStart || false}
          onChange={(_, data) => handleAutoStartToggle(data.checked)}
          className="win11-switch"
        />
      </TableCell>
      <TableCell>
        <div style={{ display: 'flex', gap: '6px', alignItems: 'center' }}>
          {service.status === 'stopped' ? (
            <Tooltip content="Start service" relationship="label">
              <Button
                size="small"
                appearance="subtle"
                icon={<Play24Regular />}
                onClick={handleStart}
                className="win11-button"
              />
            </Tooltip>
          ) : (
            <Tooltip content="Stop service" relationship="label">
              <Button
                size="small"
                appearance="secondary"
                icon={<Stop24Regular />}
                onClick={handleStop}
                className="win11-button"
              />
            </Tooltip>
          )}
          
          <Tooltip content="Delete service" relationship="label">
            <Button
              size="small"
              appearance="subtle"
              icon={<Delete24Regular />}
              onClick={handleDelete}
              className="win11-button win11-delete-button"
            />
          </Tooltip>

          <Tooltip content="Monitor service" relationship="label">
            <Button
              size="small"
              appearance="subtle"
              icon={<Desktop24Regular />}
              onClick={handleMonitor}
              className="win11-button win11-monitor-button"
            />
          </Tooltip>
        </div>
      </TableCell>
    </TableRow>
  );
});
ServiceRow.displayName = 'ServiceRow';

export default ServiceRow;