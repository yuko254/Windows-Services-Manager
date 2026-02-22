import { useState, useEffect, useRef } from 'react';
import { makeStyles, tokens, Button } from '@fluentui/react-components';

const useStyles = makeStyles({
    container: {
        fontFamily: 'Consolas, monospace',
        fontSize: tokens.fontSizeBase200,
        backgroundColor: '#1e1e1e',
        color: '#d4d4d4',
        padding: tokens.spacingHorizontalM,
        height: '400px',
        overflowY: 'auto',
        whiteSpace: 'pre-wrap',
        wordWrap: 'break-word',
        borderRadius: tokens.borderRadiusMedium,
        '& > div': {
            borderBottom: `1px solid #333`,
            lineHeight: 1.4,
            margin: 0
        }
    }
});

const ServiceLogViewer = ({ serviceId, onClose }) => {
    const styles = useStyles();
    const [lines, setLines] = useState([]);
    const containerRef = useRef(null);

    useEffect(() => {
        const handleLogLine = (data) => {
            if (data.serviceId === serviceId) {
                setLines(prev => [...prev, data.line]);
            }
        };

        const fetchInitialLogs = async () => {
            try { 
                const initialLines = await window.go.main.App.GetLogContent(serviceId);
                setLines(initialLines);
            }
            catch (error) {
                console.error('Failed to fetch initial log content:', error);
            }
        };

        fetchInitialLogs();
        const removeListener = window.runtime.EventsOn('service-log-line', handleLogLine);
        window.go.main.App.StartMonitoringService(serviceId).catch(console.error);

        return () => {
            removeListener();
            window.go.main.App.StopMonitoringService(serviceId);
            setLines([]);
        };
    }, [serviceId]);

    useEffect(() => {
        if (containerRef.current) {
            containerRef.current.scrollTop = containerRef.current.scrollHeight;
        }
    }, [lines]);

    return (
        <>
            <div ref={containerRef} className={styles.container}>
                {lines.map((line, idx) => <div key={idx}>{line}</div>)}
            </div>
            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}>
                <Button appearance="transparent" onClick={() => setLines([])}>Clear</Button>
                <Button appearance="primary" onClick={onClose}>Stop Monitoring</Button>
            </div>
        </>
    );
};

export default ServiceLogViewer;