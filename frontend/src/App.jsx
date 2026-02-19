import { useState, useEffect, useCallback, useMemo, memo, useContext } from 'react';
import { ThemeContext } from './main';
import {
  makeStyles,
  Button,
  Input,
  Table,
  TableHeader,
  TableRow,
  TableHeaderCell,
  TableBody,
  TableCell,
  Dialog,
  DialogTrigger,
  DialogSurface,
  DialogTitle,
  DialogContent,
  DialogActions,
  DialogBody,
  Field,
  Text,
  Badge,
  Toast,
  Toaster,
  useToastController,
  ToastTitle,
  Switch,
  Tooltip
} from '@fluentui/react-components';
import {
  Add24Regular,
  Play24Regular,
  Stop24Regular,
  Delete24Regular,
  Settings24Regular,
  ArrowClockwise24Regular,
  BuildingMultiple24Regular,
  Document24Regular,
  Folder24Regular
} from '@fluentui/react-icons';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';
import './App.css';
import { 
  GetServices, 
  CreateService, 
  StartService, 
  StopService, 
  DeleteService,
  SelectFile,
  SelectDirectory,
  CheckAdminPrivileges,
  SetAutoStart,
  GetAutoStartStatus,
  SetServiceAutoStart,
  RestartAsAdmin,
  AddPathVariable,
  OpenSystemEnvironmentSettings,
  ValidatePathExists,
  DiagnoseEnvironmentAccess
} from "../wailsjs/go/main/App";

const useStyles = makeStyles({
  toastGrid: {
    display: 'grid',
    gridTemplateColumns: 'auto 1fr',
    alignItems: 'start',
    gap: '8px',
  },
  messageColumn: {
    gridColumn: 2,
    gridRow: 2,
  },
});

// Service row component, optimized with memo
const ServiceRow = memo(({ service, onStart, onStop, onDelete, onAutoStartToggle }) => {
  const handleStart = useCallback(() => onStart(service.id), [service.id, onStart]);
  const handleStop = useCallback(() => onStop(service.id), [service.id, onStop]);
  const handleDelete = useCallback(() => onDelete(service.id), [service.id, onDelete]);
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
        </div>
      </TableCell>
    </TableRow>
  );
});
ServiceRow.displayName = 'ServiceRow';

function App() {
  const { isDark, toggleTheme } = useContext(ThemeContext);
  const [services, setServices] = useState([]);
  const [isAddDialogOpen, setIsAddDialogOpen] = useState(false);
  const [isSettingsDialogOpen, setIsSettingsDialogOpen] = useState(false);
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
  const [isEnvDialogOpen, setIsEnvDialogOpen] = useState(false);
  const [serviceToDelete, setServiceToDelete] = useState(null);
  const [adminPrivileges, setAdminPrivileges] = useState(false);
  const [autoStart, setAutoStart] = useState(false);
  const [showAdminWarning, setShowAdminWarning] = useState(false);
  const [envPath, setEnvPath] = useState('');
  const [isAddingEnv, setIsAddingEnv] = useState(false);
  const [newService, setNewService] = useState({
    name: '',
    exePath: '',
    args: '',
    workingDir: ''
  });

  const styles = useStyles();
  const { dispatchToast } = useToastController();

  const showToast = useCallback((title, message, intent = 'success') => {
    dispatchToast(
      <Toast className={styles.toastGrid}>
          <ToastTitle>{title}</ToastTitle>
          {message && <Text className={styles.messageColumn}>{message}</Text>}
      </Toast>,
      { intent, timeout: 3000 }
    );
  }, [dispatchToast]);

  useEffect(() => {
    loadServices();
    checkAdminRights();
    checkAutoStartStatus();
    
    // Listen for service status change events
    EventsOn('service-status-changed', (data) => {
      setServices(prev => prev.map(service => 
        service.id === data.serviceId 
          ? { ...service, status: data.status, pid: data.pid }
          : service
      ));
    });
    
    // Listen for service list update events
    EventsOn('services-updated', (serviceList) => {
      setServices(serviceList || []);
    });
    
    return () => {
      EventsOff('service-status-changed');
      EventsOff('services-updated');
    };
  }, []);

  const checkAdminRights = useCallback(async () => {
    try {
      const isAdmin = await CheckAdminPrivileges();
      setAdminPrivileges(isAdmin);
      if (!isAdmin) {
        setShowAdminWarning(true);
      }
    } catch (error) {
      console.error('Failed to check permissions:', error);
    }
  }, []);

  const checkAutoStartStatus = useCallback(async () => {
    try {
      const status = await GetAutoStartStatus();
      setAutoStart(status);
    } catch (error) {
      console.error('Failed to check auto-start status:', error);
    }
  }, []);

  const handleAppAutoStartToggle = useCallback(async (enabled) => {
    try {
      await SetAutoStart(enabled);
      setAutoStart(enabled);
      showToast('Success', `Auto-start ${enabled ? 'enabled' : 'disabled'}`);
    } catch (error) {
      showToast('Error', 'Failed to set auto-start: ' + error, 'error');
    }
  }, [showToast]);

  const handleRestartAsAdmin = useCallback(async () => {
    try {
      await RestartAsAdmin();
    } catch (error) {
      showToast('Error', 'Failed to restart as administrator: ' + error, 'error');
    }
  }, [showToast]);

  const loadServices = useCallback(async () => {
    try {
      const serviceList = await GetServices();
      setServices(serviceList || []);
    } catch (error) {
      showToast('Error', 'Failed to load service list: ' + error, 'error');
    }
  }, [showToast]);

  const handleCreateService = useCallback(async () => {
    if (!newService.name || !newService.exePath) {
      showToast('Validation error', 'Please enter service name and executable path', 'error');
      return;
    }

    try {
      await CreateService(newService);
      showToast('Success', 'Service created successfully');
      setIsAddDialogOpen(false);
      setNewService({
        name: '',
        exePath: '',
        args: '',
        workingDir: ''
      });
      loadServices();
    } catch (error) {
      showToast('Error', 'Failed to create service: ' + error, 'error');
    }
  }, [newService, showToast, loadServices]);

  const handleStartService = useCallback(async (serviceId) => {
    try {
      await StartService(serviceId);
      showToast('Success', 'Service started successfully');
      loadServices();
    } catch (error) {
      showToast('Error', 'Failed to start service: ' + error, 'error');
    }
  }, [showToast, loadServices]);

  const handleStopService = useCallback(async (serviceId) => {
    try {
      await StopService(serviceId);
      showToast('Success', 'Service stopped successfully');
      loadServices();
    } catch (error) {
      showToast('Error', 'Failed to stop service: ' + error, 'error');
    }
  }, [showToast, loadServices]);

  const handleDeleteService = useCallback((serviceId) => {
    const service = services.find(s => s.id === serviceId);
    setServiceToDelete(service);
    setIsDeleteDialogOpen(true);
  }, [services]);

  const confirmDeleteService = useCallback(async () => {
    if (!serviceToDelete) return;
    
    try {
      await DeleteService(serviceToDelete.id);
      showToast('Success', 'Service deleted successfully');
      loadServices();
    } catch (error) {
      showToast('Error', 'Failed to delete service: ' + error, 'error');
    } finally {
      setIsDeleteDialogOpen(false);
      setServiceToDelete(null);
    }
  }, [serviceToDelete, showToast, loadServices]);

  const handleAutoStartToggle = useCallback(async (serviceId, enabled) => {
    try {
      await SetServiceAutoStart(serviceId, enabled);
      showToast('Success', enabled ? 'Auto-start enabled' : 'Auto-start disabled');
      loadServices();
    } catch (error) {
      showToast('Error', 'Failed to set auto-start: ' + error, 'error');
    }
  }, [showToast, loadServices]);

  const handleSelectFile = useCallback(async () => {
    try {
      const filePath = await SelectFile();
      if (filePath) {
        setNewService(prev => ({ ...prev, exePath: filePath }));
      }
    } catch (error) {
      showToast('Error', 'Failed to select file: ' + error, 'error');
    }
  }, [showToast]);

  const handleSelectDirectory = useCallback(async () => {
    try {
      const dirPath = await SelectDirectory();
      if (dirPath) {
        setNewService(prev => ({ ...prev, workingDir: dirPath }));
      }
    } catch (error) {
      showToast('Error', 'Failed to select directory: ' + error, 'error');
    }
  }, [showToast]);

  const handleSelectEnvFile = useCallback(async () => {
    try {
      const filePath = await SelectFile();
      if (filePath) {
        setEnvPath(filePath);
      }
    } catch (error) {
      showToast('Error', 'Failed to select file: ' + error, 'error');
    }
  }, [showToast]);

  const handleSelectEnvDirectory = useCallback(async () => {
    try {
      const dirPath = await SelectDirectory();
      if (dirPath) {
        setEnvPath(dirPath);
      }
    } catch (error) {
      showToast('Error', 'Failed to select directory: ' + error, 'error');
    }
  }, [showToast]);

  const handleAddEnvironmentVariable = useCallback(async () => {
    if (!envPath.trim()) {
      showToast('Validation Error', 'Please enter or select a file path', 'error');
      return;
    }

    setIsAddingEnv(true);
    try {
      // Validate if path exists
      const exists = await ValidatePathExists(envPath);
      if (!exists) {
        showToast('Validation Error', 'The specified path does not exist', 'error');
        return;
      }

      // Add to PATH environment variable
      await AddPathVariable(envPath);
      showToast('Success', 'PATH environment variable added successfully! It will take effect in new command prompt windows.');
      
      // Close dialog and clear input
      setIsEnvDialogOpen(false);
      setEnvPath('');
    } catch (error) {
      console.error('Failed to add environment variable:', error);
      
      // If it's a permission error, perform diagnostics
      if (error.toString().includes('Access is denied') || 
          error.toString().includes('access denied') ||
          error.toString().includes('cannot read existing PATH variable')) {
        
        try {
          const diagnosis = await DiagnoseEnvironmentAccess();
          console.log('Permission diagnosis result:', diagnosis);
          
          let errorMsg = 'Insufficient permissions to modify system environment variables.\n\n';
          
          if (!diagnosis.registry_full) {
            errorMsg += '• Registry full access: Failed\n';
          }
          if (!diagnosis.registry_write) {
            errorMsg += '• Registry write access: Failed\n';
          }
          if (!diagnosis.path_read) {
            errorMsg += '• PATH variable read: Failed\n';
          }
          
          errorMsg += '\nPlease ensure:\n';
          errorMsg += '1. The program is running as administrator\n';
          errorMsg += '2. System group policies do not restrict environment variable changes\n';
          errorMsg += '3. Antivirus software is not blocking registry access';
          
          showToast('Permission Diagnosis', errorMsg, 'error');
        } catch (diagError) {
          showToast('Error', 'Failed to add environment variable: ' + error + '\nDiagnosis failed: ' + diagError, 'error');
        }
      } else {
        showToast('Error', 'Failed to add environment variable: ' + error, 'error');
      }
    } finally {
      setIsAddingEnv(false);
    }
  }, [envPath, showToast]);

  const handleOpenSystemEnvironmentSettings = useCallback(async () => {
    try {
      await OpenSystemEnvironmentSettings();
    } catch (error) {
      showToast('Error', 'Failed to open system environment settings: ' + error, 'error');
    }
  }, [showToast]);


  const columns = useMemo(() => [
    { columnKey: 'name', label: 'Service Name' },
    { columnKey: 'status', label: 'Status' },
    { columnKey: 'exePath', label: 'Program Path' },
    { columnKey: 'autoStart', label: 'Auto Start' },
    { columnKey: 'actions', label: 'Actions' }
  ], []);

  const serviceStats = useMemo(() => ({
    total: services.length,
    running: services.filter(s => s.status === 'running').length,
    stopped: services.filter(s => s.status === 'stopped').length
  }), [services]);

  return (
    <>
      <Toaster />
      <div className="app-container">
        <div className="header">
          <Text size="400" weight="semibold">Windows Service Manager</Text>
          <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '10px' }}>
            {!adminPrivileges && (
              <Badge color="warning" appearance="filled" className="win11-badge">Non-Admin Mode</Badge>
            )}
            <Button 
              appearance="subtle" 
              icon={<BuildingMultiple24Regular />}
              onClick={() => setIsEnvDialogOpen(true)}
              className="win11-button"
            >
              System Variables
            </Button>
            <Button 
              appearance="subtle" 
              icon={<Settings24Regular />}
              onClick={() => setIsSettingsDialogOpen(true)}
              className="win11-button"
            >
              App Settings
            </Button>
            <Dialog open={isAddDialogOpen} onOpenChange={(_, data) => setIsAddDialogOpen(data.open)}>
              <DialogTrigger disableButtonEnhancement>
                <Button appearance="primary" icon={<Add24Regular />} className="win11-button">
                  Add Service
                </Button>
              </DialogTrigger>
              <DialogSurface className="win11-dialog">
                <DialogBody>
                  <DialogTitle>Add New Service</DialogTitle>
                  <DialogContent>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
                      <Field label="Service Name" required>
                        <Input
                          value={newService.name}
                          onChange={(e) => setNewService(prev => ({ ...prev, name: e.target.value }))}
                          placeholder="Enter service name"
                          className="win11-input"
                        />
                      </Field>
                      
                      <Field label="Executable Path" required>
                        <div style={{ display: 'flex', gap: '8px' }}>
                          <Input
                            value={newService.exePath}
                            onChange={(e) => setNewService(prev => ({ ...prev, exePath: e.target.value }))}
                            placeholder="Enter program path"
                            style={{ flex: 1 }}
                            className="win11-input"
                          />
                          <Button 
                            icon={<Document24Regular />} 
                            onClick={handleSelectFile}
                            className="win11-button"
                          >
                            Browse
                          </Button>
                        </div>
                      </Field>
                      
                      <Field label="Startup Parameters">
                        <Input
                          value={newService.args}
                          onChange={(e) => setNewService(prev => ({ ...prev, args: e.target.value }))}
                          placeholder="Enter startup parameters (optional)"
                          className="win11-input"
                        />
                      </Field>
                      
                      <Field label="Working Directory">
                        <div style={{ display: 'flex', gap: '8px' }}>
                          <Input
                            value={newService.workingDir}
                            onChange={(e) => setNewService(prev => ({ ...prev, workingDir: e.target.value }))}
                            placeholder="Working directory (leave empty to use program directory)"
                            style={{ flex: 1 }}
                            className="win11-input"
                          />
                          <Button 
                            icon={<Folder24Regular />} 
                            onClick={handleSelectDirectory}
                            className="win11-button"
                          >
                            Browse
                          </Button>
                        </div>
                      </Field>
                      
                      <Field label="Service Startup">
                        <Text size="300" style={{ 
                          color: '#666', 
                          fontStyle: 'italic',
                          padding: '8px 12px',
                          backgroundColor: '#f3f4f6',
                          borderRadius: '6px',
                          border: '1px solid #e5e7eb'
                        }}>
                          💡 The service will automatically start after creation
                        </Text>
                      </Field>
                    </div>
                  </DialogContent>
                  <DialogActions>
                    <DialogTrigger disableButtonEnhancement>
                      <Button appearance="secondary" className="win11-button">Cancel</Button>
                    </DialogTrigger>
                    <Button appearance="primary" onClick={handleCreateService} className="win11-button">
                      Create Service
                    </Button>
                  </DialogActions>
                </DialogBody>
              </DialogSurface>
            </Dialog>
          </div>
        </div>

        {/* Admin Warning Dialog */}
        <Dialog open={showAdminWarning} modalType="alert">
          <DialogSurface className="win11-dialog">
            <DialogBody>
              <DialogTitle>Admin Privileges Warning</DialogTitle>
              <DialogContent>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '16px', textAlign: 'center' }}>
                  <Text size="400" weight="semibold" style={{ color: '#d13438' }}>
                    No administrator privileges – service management features are unavailable!
                  </Text>
                  <Text size="300">
                    Please restart the program with administrator privileges for full functionality.
                  </Text>
                </div>
              </DialogContent>
              <DialogActions>
                <Button 
                  appearance="primary" 
                  onClick={handleRestartAsAdmin}
                  className="win11-button"
                >
                  Restart as Administrator
                </Button>
                <Button 
                  appearance="secondary" 
                  onClick={() => setShowAdminWarning(false)}
                  className="win11-button"
                >
                  Ignore
                </Button>
              </DialogActions>
            </DialogBody>
          </DialogSurface>
        </Dialog>

        {/* Settings Dialog */}
        <Dialog open={isSettingsDialogOpen} onOpenChange={(_, data) => setIsSettingsDialogOpen(data.open)}>
          <DialogSurface className="win11-dialog">
            <DialogBody>
              <DialogTitle>App Settings</DialogTitle>
              <DialogContent>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
                  <Field label="Privileges Management">
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <Text>Current privilege level</Text>
                        <Badge 
                          color={adminPrivileges ? "success" : "warning"} 
                          appearance="filled"
                          className="win11-badge"
                        >
                          {adminPrivileges ? "Administrator" : "Standard User"}
                        </Badge>
                      </div>
                      {!adminPrivileges && (
                        <Button 
                          appearance="primary" 
                          size="small"
                          onClick={handleRestartAsAdmin}
                          className="win11-button"
                        >
                          Restart as Administrator
                        </Button>
                      )}
                    </div>
                  </Field>

                  <Field label="Auto-start with Windows">
                    <div style={{ 
                      display: 'flex', 
                      justifyContent: 'space-between', 
                      alignItems: 'center',
                      padding: '12px 16px',
                      backgroundColor: 'rgba(255, 255, 255, 0.5)',
                      borderRadius: '12px',
                      backdropFilter: 'blur(10px)'
                    }}>
                      <Text>Add this program to auto-start at boot</Text>
                      <Switch
                        checked={autoStart}
                        onChange={(_, data) => handleAppAutoStartToggle(data.checked)}
                      />
                    </div>
                  </Field>

                  <Field>
                    <div style={{ 
                      display: 'flex', 
                      justifyContent: 'space-between', 
                      alignItems: 'center',
                      padding: '12px 16px',
                      backgroundColor: 'rgba(255, 255, 255, 0.5)',
                      borderRadius: '12px',
                      backdropFilter: 'blur(10px)'
                    }}>
                      <Text>Dark theme</Text>
                      <Switch
                        checked={isDark}
                        onChange={(_, data) => toggleTheme(data.checked)}
                      />
                    </div>
                  </Field>

                  <Field label="App Info">
                    <div style={{ 
                      display: 'flex', 
                      flexDirection: 'column', 
                      gap: '8px',
                      padding: '16px',
                      backgroundColor: 'rgba(255, 255, 255, 0.5)',
                      borderRadius: '12px',
                      backdropFilter: 'blur(10px)'
                    }}>
                      <Text size="300" weight="semibold">Windows Service Manager</Text>
                      <Text size="200" style={{ color: '#666' }}>Modern Windows service management tool</Text>
                      <Text size="200" style={{ color: '#666' }}>Run any program as a background service</Text>
                      <Text size="200" style={{ color: '#666' }}>Project URL: https://github.com/sky22333/services</Text>
                    </div>
                  </Field>
                </div>
              </DialogContent>
              <DialogActions>
                <Button 
                  appearance="primary" 
                  onClick={() => setIsSettingsDialogOpen(false)}
                  className="win11-button"
                >
                  Close
                </Button>
              </DialogActions>
            </DialogBody>
          </DialogSurface>
        </Dialog>

        {/* System Variables Dialog */}
        <Dialog open={isEnvDialogOpen} onOpenChange={(_, data) => setIsEnvDialogOpen(data.open)}>
          <DialogSurface className="win11-dialog">
            <DialogBody>
              <DialogTitle>Add System Environment Variable</DialogTitle>
              <DialogContent>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '18px' }}>
                  <Field label="File or Directory Path" required>
                    <div style={{ 
                      display: 'flex', 
                      flexDirection: 'column', 
                      gap: '12px',
                      padding: '16px',
                      backgroundColor: 'rgba(255, 255, 255, 0.5)',
                      borderRadius: '12px',
                      backdropFilter: 'blur(10px)',
                      border: '1px solid #e5e7eb'
                    }}>
                      <Text size="300" style={{ color: '#666', marginBottom: '8px' }}>
                        💡 Enter or select the file/directory path to add to the system PATH
                      </Text>
                      
                      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
                        <Input
                          value={envPath}
                          onChange={(e) => setEnvPath(e.target.value)}
                          placeholder="e.g., C:\Program Files\MyApp\bin"
                          style={{ flex: 1 }}
                          className="win11-input"
                        />
                        <div style={{ display: 'flex', gap: '4px' }}>
                          <Tooltip content="Select executable file (extracts directory automatically)" relationship="label">
                            <Button 
                              icon={<Document24Regular />} 
                              onClick={handleSelectEnvFile}
                              className="win11-button"
                              size="small"
                            >
                              File
                            </Button>
                          </Tooltip>
                          <Tooltip content="Select directory directly" relationship="label">
                            <Button 
                              icon={<Folder24Regular />} 
                              onClick={handleSelectEnvDirectory}
                              className="win11-button"
                              size="small"
                              appearance="secondary"
                            >
                              Directory
                            </Button>
                          </Tooltip>
                        </div>
                      </div>
                      
                      <div style={{ 
                        fontSize: '12px', 
                        color: '#666',
                        padding: '8px 12px',
                        backgroundColor: '#f8f9fa',
                        borderRadius: '6px',
                        border: '1px solid #e9ecef'
                      }}>
                        <div><strong>Function:</strong></div>
                        <div><strong>Description:</strong> Quickly add programs to system variables</div>
                        <div><strong>Usage:</strong> Supports manual path entry or selection of a program/directory</div>
                        <div><strong>Effect:</strong> Path is added to the system PATH; it becomes available in new terminal windows</div>
                      </div>
                    </div>
                  </Field>

                  <Field label="Quick Actions">
                    <div style={{ 
                      display: 'flex', 
                      gap: '12px',
                      padding: '12px 16px',
                      backgroundColor: 'rgba(255, 255, 255, 0.5)',
                      borderRadius: '12px',
                      backdropFilter: 'blur(10px)'
                    }}>
                      <Button
                        appearance="secondary"
                        onClick={handleOpenSystemEnvironmentSettings}
                        className="win11-button"
                        size="small"
                      >
                        Open System Environment Settings
                      </Button>
                    </div>
                  </Field>
                </div>
              </DialogContent>
              <DialogActions>
                <Button 
                  appearance="secondary" 
                  onClick={() => {
                    setIsEnvDialogOpen(false);
                    setEnvPath('');
                  }}
                  className="win11-button"
                >
                  Cancel
                </Button>
                <Button 
                  appearance="primary" 
                  onClick={handleAddEnvironmentVariable}
                  disabled={!envPath.trim() || isAddingEnv}
                  className="win11-button"
                >
                  {isAddingEnv ? 'Adding...' : 'Add to PATH'}
                </Button>
              </DialogActions>
            </DialogBody>
          </DialogSurface>
        </Dialog>

        <div className="main-content">
          <div className="content-area">
            <div style={{ 
              display: 'flex', 
              justifyContent: 'space-between', 
              alignItems: 'center',
              marginBottom: '16px'
            }}>
              <Text size="300" weight="semibold">Service List</Text>
              <Button 
                appearance="subtle" 
                icon={<ArrowClockwise24Regular />}
                onClick={loadServices}
                className="win11-button"
              >
                Refresh
              </Button>
            </div>
            
            {services.length === 0 ? (
              <div className="empty-state">
                <div className="empty-state-icon">⚙️</div>
                <div className="empty-state-text">
                  No services<br />
                  Click the "Add Service" button in the top right to get started
                </div>
              </div>
            ) : (
              <Table className="win11-table slide-in">
                <TableHeader className="win11-table-header">
                  <TableRow>
                    {columns.map(col => (
                      <TableHeaderCell key={col.columnKey}>
                        <Text weight="semibold">{col.label}</Text>
                      </TableHeaderCell>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {services.map(service => (
                    <ServiceRow
                      key={service.id}
                      service={service}
                      onStart={handleStartService}
                      onStop={handleStopService}
                      onDelete={handleDeleteService}
                      onAutoStartToggle={handleAutoStartToggle}
                    />
                  ))}
                </TableBody>
              </Table>
            )}
          </div>
        </div>

        <div className="status-bar">
          <Text size="200">
            Total services: {serviceStats.total} | 
            Running: {serviceStats.running} | 
            Stopped: {serviceStats.stopped}
            {adminPrivileges ? ' | Administrator' : ' | Standard user'}
          </Text>
        </div>
      </div>

      {/* Delete Confirmation Dialog */}
      <Dialog open={isDeleteDialogOpen} onOpenChange={(_, data) => setIsDeleteDialogOpen(data.open)}>
        <DialogSurface>
          <DialogBody>
            <DialogTitle>Confirm Delete Service</DialogTitle>
            <DialogContent>
              <Text>
                Are you sure you want to delete the service "{serviceToDelete?.name}"?
              </Text>
              <br />
              <Text style={{ marginTop: '8px', color: '#d13438' }}>
                This service will be permanently removed!
              </Text>
            </DialogContent>
            <DialogActions>
              <Button 
                appearance="secondary" 
                onClick={() => setIsDeleteDialogOpen(false)}
              >
                Cancel
              </Button>
              <Button 
                appearance="primary" 
                onClick={confirmDeleteService}
                style={{ backgroundColor: '#d13438', borderColor: '#d13438' }}
              >
                Delete
              </Button>
            </DialogActions>
          </DialogBody>
        </DialogSurface>
      </Dialog>
    </>
  );
}

export default memo(App);