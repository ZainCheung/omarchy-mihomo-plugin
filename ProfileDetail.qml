import QtQuick
import QtQuick.Controls
import qs.Ui
import qs.Commons

Popup {
  id: root
  property var svc: null
  property string profileId: ""
  property string profileName: ""
  property bool globalScope: false
  property bool showRuntime: false
  property bool showOverride: false
  property string runtimeText: ""
  property color foreground: Color.popups.text
  property string fontFamily: Style.font.family
  modal: true; focus: true; padding: Style.space(18); width: Style.space(500); height: Style.space(460); anchors.centerIn: Overlay.overlay
  background: Rectangle { color: Color.popups.background; radius: Style.cornerRadius; border.width: 1; border.color: Util.alpha(root.foreground,0.2) }
  Column { anchors.fill: parent; spacing: Style.space(8)
    Row { width: parent.width; spacing: Style.space(8)
      Text { text: root.globalScope ? (root.svc ? root.svc.t("globalOverride") : "Global Override") : root.showRuntime ? (root.svc ? root.svc.t("runtimeConfig") : "Runtime") : root.showOverride ? (root.svc ? root.svc.t("profileOverride") : "Profile Override") : (root.svc ? root.svc.t("profileSource") : "Source Config"); color: root.foreground; font.family: root.fontFamily; font.pixelSize: Style.font.subtitle; font.bold: true }
      Text { text: root.svc && root.svc.activeProfile === root.profileId ? root.svc.t("activeProfile") : ""; color: Color.accent; font.family: root.fontFamily; font.pixelSize: Style.font.caption }
      Button { visible: !root.globalScope; text: root.showOverride ? (root.svc ? root.svc.t("viewSource") : "View Source") : (root.svc ? root.svc.t("viewOverride") : "View Override"); onClicked: { root.showRuntime = false; root.showOverride = !root.showOverride; root.load() } }
      Button { visible: !root.globalScope && !root.showOverride; text: root.showRuntime ? (root.svc ? root.svc.t("profileSource") : "Source") : (root.svc ? root.svc.t("runtimeConfig") : "Runtime"); onClicked: { root.showRuntime = !root.showRuntime; root.showOverride = false; root.load() } }
      Button { visible: root.showOverride && !root.globalScope; text: root.svc ? root.svc.t("editOverride") : "Edit Override"; onClicked: root.svc.openProfileOverride(root.profileId) }
      Button { visible: root.globalScope; text: root.svc ? root.svc.t("editOverride") : "Edit Override"; onClicked: root.svc.openGlobalOverride() }
    }
    TextArea { id: source; width: parent.width; height: parent.height-Style.space(48); readOnly: true; wrapMode: TextArea.NoWrap; text: root.svc ? root.svc.t("loading") : "Loading…"; color: root.foreground; font.family: "monospace"; selectByMouse: true }
    Button { text: root.svc ? root.svc.t("close") : "Close"; anchors.right: parent.right; onClicked: root.close() }
  }
  function load() {
    if (!root.svc || (!root.globalScope && root.profileId === "")) return
    if (root.globalScope) {
      source.text = root.svc.t("loading")
      root.svc.readGlobalOverride(function(text) { source.text = text })
    } else if (root.showOverride) {
      source.text = root.svc ? root.svc.t("loading") : "Loading…"
      root.svc.readProfileOverride(root.profileId, function(text) { source.text = text })
    } else if (root.showRuntime) {
      source.text = root.svc ? root.svc.t("loading") : "Loading…"
      if (root.svc && root.svc.activeProfile === root.profileId) {
        root.svc.readProfileRuntime(root.profileId, function(text) { root.runtimeText = text; source.text = text })
      } else {
        source.text = root.svc.t("runtimeUnavailable")
      }
    } else {
      source.text = root.svc ? root.svc.t("loading") : "Loading…"
      root.svc.readProfileSource(root.profileId, function(text) { source.text = text })
    }
  }
  onOpened: load()
}
