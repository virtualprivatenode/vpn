// internal/installer/syncthing_template.go

package installer

import "strings"

// syncthingConfigSchema is the config schema version this
// template is authored against. Tied to syncthingVersion
// (setup.go): bumping the pinned Syncthing version without
// re-reviewing this template against the new version's
// `syncthing generate` output MUST fail the install — that is
// the version tripwire (verifySyncthingConfig, gate a/b).
const syncthingConfigSchema = "52"

// syncthingConfigTemplate is the COMPLETE authored config.xml,
// written over the `syncthing generate` output before first
// daemon start. Source of truth: verbatim `syncthing generate`
// output of the pinned v2.1.1 GitHub binary (captured June 9
// 2026), with these deliberate deltas ONLY:
//
//	Privacy (finding H field set — each verified by the
//	self-verify gate):
//	  listenAddress        default → tcp://0.0.0.0:22000 (single;
//	                       kills QUIC/UDP+STUN and relay listeners)
//	  globalAnnounceEnabled true → false   (no discovery announce)
//	  localAnnounceEnabled  true → false   (no LAN broadcast)
//	  relaysEnabled         true → false   (no relay registration)
//	  natEnabled            true → false   (no UPnP/NAT-PMP)
//	  announceLANAddresses  true → false
//	  crashReportingEnabled true → false   (no crash.syncthing.net)
//	  autoUpgradeIntervalH  12 → 0         (self-upgrade off;
//	                       pairs with STNOUPGRADE=1 in the unit)
//	  urAccepted            0 → -1         (usage reporting
//	                       declined; no data.syncthing.net POSTs)
//
//	Auth (existing behavior, now authored):
//	  gui user=admin, password=bcrypt hash,
//	  insecureSkipHostcheck=true (required for the Tor onion GUI:
//	  the browser sends Host: <onion>, which the host check
//	  would reject; rebinding risk N/A in this topology)
//
//	Omitted: <unackedNotificationID> (auth-setup nag; credentials
//	always exist before first start).
//
// Every other field is generate-output verbatim, kept at default
// deliberately. Per-field rationale doc: finding T work item.
//
// Placeholders (strings.NewReplacer, NOT fmt.Sprintf — the
// template contains literal '%' in unit="%" attributes):
//
//	{{DEVICE_ID}}    device ID from the generated TLS cert
//	{{DEVICE_NAME}}  device name from the generated config
//	{{API_KEY}}      API key from the generated config
//	{{PASSWORD_HASH}} bcrypt hash of the GUI password
const syncthingConfigTemplate = `<configuration version="52">
    <device id="{{DEVICE_ID}}" name="{{DEVICE_NAME}}" compression="metadata" introducer="false" skipIntroductionRemovals="false" introducedBy="">
        <address>dynamic</address>
        <paused>false</paused>
        <autoAcceptFolders>false</autoAcceptFolders>
        <maxSendKbps>0</maxSendKbps>
        <maxRecvKbps>0</maxRecvKbps>
        <maxRequestKiB>0</maxRequestKiB>
        <untrusted>false</untrusted>
        <remoteGUIPort>0</remoteGUIPort>
        <numConnections>0</numConnections>
    </device>
    <gui enabled="true" tls="false" sendBasicAuthPrompt="false">
        <address>127.0.0.1:8384</address>
        <user>admin</user>
        <password>{{PASSWORD_HASH}}</password>
        <metricsWithoutAuth>false</metricsWithoutAuth>
        <apikey>{{API_KEY}}</apikey>
        <insecureSkipHostcheck>true</insecureSkipHostcheck>
        <theme>default</theme>
        <sessionCookieDurationS>604800</sessionCookieDurationS>
        <sessionCookiePath>/</sessionCookiePath>
    </gui>
    <ldap></ldap>
    <options>
        <listenAddress>tcp://0.0.0.0:22000</listenAddress>
        <globalAnnounceServer>default</globalAnnounceServer>
        <globalAnnounceEnabled>false</globalAnnounceEnabled>
        <localAnnounceEnabled>false</localAnnounceEnabled>
        <localAnnouncePort>21027</localAnnouncePort>
        <localAnnounceMCAddr>[ff12::8384]:21027</localAnnounceMCAddr>
        <maxSendKbps>0</maxSendKbps>
        <maxRecvKbps>0</maxRecvKbps>
        <reconnectionIntervalS>20</reconnectionIntervalS>
        <relaysEnabled>false</relaysEnabled>
        <relayReconnectIntervalM>10</relayReconnectIntervalM>
        <startBrowser>true</startBrowser>
        <natEnabled>false</natEnabled>
        <natLeaseMinutes>60</natLeaseMinutes>
        <natRenewalMinutes>30</natRenewalMinutes>
        <natTimeoutSeconds>10</natTimeoutSeconds>
        <urAccepted>-1</urAccepted>
        <urSeen>0</urSeen>
        <urUniqueID></urUniqueID>
        <urURL>https://data.syncthing.net/newdata</urURL>
        <urPostInsecurely>false</urPostInsecurely>
        <urInitialDelayS>1800</urInitialDelayS>
        <autoUpgradeIntervalH>0</autoUpgradeIntervalH>
        <upgradeToPreReleases>false</upgradeToPreReleases>
        <keepTemporariesH>24</keepTemporariesH>
        <cacheIgnoredFiles>false</cacheIgnoredFiles>
        <progressUpdateIntervalS>5</progressUpdateIntervalS>
        <limitBandwidthInLan>false</limitBandwidthInLan>
        <minHomeDiskFree unit="%">1</minHomeDiskFree>
        <releasesURL>https://upgrades.syncthing.net/meta.json</releasesURL>
        <overwriteRemoteDeviceNamesOnConnect>false</overwriteRemoteDeviceNamesOnConnect>
        <tempIndexMinBlocks>10</tempIndexMinBlocks>
        <trafficClass>0</trafficClass>
        <setLowPriority>true</setLowPriority>
        <maxFolderConcurrency>0</maxFolderConcurrency>
        <crashReportingURL>https://crash.syncthing.net/newcrash</crashReportingURL>
        <crashReportingEnabled>false</crashReportingEnabled>
        <stunKeepaliveStartS>180</stunKeepaliveStartS>
        <stunKeepaliveMinS>20</stunKeepaliveMinS>
        <stunServer>default</stunServer>
        <maxConcurrentIncomingRequestKiB>0</maxConcurrentIncomingRequestKiB>
        <announceLANAddresses>false</announceLANAddresses>
        <sendFullIndexOnUpgrade>false</sendFullIndexOnUpgrade>
        <auditEnabled>false</auditEnabled>
        <auditFile></auditFile>
        <connectionLimitEnough>0</connectionLimitEnough>
        <connectionLimitMax>0</connectionLimitMax>
        <connectionPriorityTcpLan>10</connectionPriorityTcpLan>
        <connectionPriorityQuicLan>20</connectionPriorityQuicLan>
        <connectionPriorityTcpWan>30</connectionPriorityTcpWan>
        <connectionPriorityQuicWan>40</connectionPriorityQuicWan>
        <connectionPriorityRelay>50</connectionPriorityRelay>
        <connectionPriorityUpgradeThreshold>0</connectionPriorityUpgradeThreshold>
    </options>
    <defaults>
        <folder id="" label="" path="" type="sendreceive" rescanIntervalS="3600" fsWatcherEnabled="true" fsWatcherDelayS="10" fsWatcherTimeoutS="0" ignorePerms="false" autoNormalize="true">
            <filesystemType>basic</filesystemType>
            <device id="{{DEVICE_ID}}" introducedBy="">
                <encryptionPassword></encryptionPassword>
            </device>
            <minDiskFree unit="%">1</minDiskFree>
            <versioning>
                <cleanupIntervalS>3600</cleanupIntervalS>
                <fsPath></fsPath>
                <fsType>basic</fsType>
            </versioning>
            <copiers>0</copiers>
            <pullerMaxPendingKiB>0</pullerMaxPendingKiB>
            <hashers>0</hashers>
            <order>random</order>
            <ignoreDelete>false</ignoreDelete>
            <scanProgressIntervalS>0</scanProgressIntervalS>
            <pullerPauseS>0</pullerPauseS>
            <pullerDelayS>1</pullerDelayS>
            <maxConflicts>10</maxConflicts>
            <disableSparseFiles>false</disableSparseFiles>
            <paused>false</paused>
            <markerName>.stfolder</markerName>
            <copyOwnershipFromParent>false</copyOwnershipFromParent>
            <modTimeWindowS>0</modTimeWindowS>
            <maxConcurrentWrites>16</maxConcurrentWrites>
            <disableFsync>false</disableFsync>
            <blockPullOrder>standard</blockPullOrder>
            <copyRangeMethod>standard</copyRangeMethod>
            <caseSensitiveFS>false</caseSensitiveFS>
            <junctionsAsDirs>false</junctionsAsDirs>
            <syncOwnership>false</syncOwnership>
            <sendOwnership>false</sendOwnership>
            <syncXattrs>false</syncXattrs>
            <sendXattrs>false</sendXattrs>
            <blockIndexing>true</blockIndexing>
            <xattrFilter>
                <maxSingleEntrySize>1024</maxSingleEntrySize>
                <maxTotalSize>4096</maxTotalSize>
            </xattrFilter>
        </folder>
        <device id="" compression="metadata" introducer="false" skipIntroductionRemovals="false" introducedBy="">
            <address>dynamic</address>
            <paused>false</paused>
            <autoAcceptFolders>false</autoAcceptFolders>
            <maxSendKbps>0</maxSendKbps>
            <maxRecvKbps>0</maxRecvKbps>
            <maxRequestKiB>0</maxRequestKiB>
            <untrusted>false</untrusted>
            <remoteGUIPort>0</remoteGUIPort>
            <numConnections>0</numConnections>
        </device>
        <ignores></ignores>
    </defaults>
</configuration>
`

func renderSyncthingConfig(
	deviceID, deviceName, apiKey, passwordHash string,
) string {
	return strings.NewReplacer(
		"{{DEVICE_ID}}", deviceID,
		"{{DEVICE_NAME}}", deviceName,
		"{{API_KEY}}", apiKey,
		"{{PASSWORD_HASH}}", passwordHash,
	).Replace(syncthingConfigTemplate)
}
