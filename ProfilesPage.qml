import QtQuick
import QtQuick.Controls
import qs.Ui
import qs.Commons

Item {
  id: root
  property var svc: null
  property color fg: Color.popups.text
  property string fontFamily: Style.font.family

  AddProfileDialog {
    id: addDialog
    svc: root.svc
    foreground: root.fg
    fontFamily: root.fontFamily
  }

  AddProfileDialog {
    id: importDialog
    svc: root.svc
    mode: "local"
    foreground: root.fg
    fontFamily: root.fontFamily
  }

  AddProfileDialog {
    id: renameDialog
    svc: root.svc
    mode: "rename"
    foreground: root.fg
    fontFamily: root.fontFamily
  }

  AddProfileDialog {
    id: urlDialog
    svc: root.svc
    mode: "url"
    foreground: root.fg
    fontFamily: root.fontFamily
  }

  ProfileDetail {
    id: detail
    svc: root.svc
    profileId: ""
    foreground: root.fg
    fontFamily: root.fontFamily
  }

  property bool profileRemoving: false
  property string profileRemoveId: ""
  property string profileRemoveName: ""
  ConfirmDialog {
    id: deleteConfirm
    anchors.fill: parent
    z: 100
    opened: root.profileRemoving
    message: root.svc ? root.svc.t("deleteProfileConfirm", root.profileRemoveName) : "Delete this profile?"
    cancelText: root.svc ? root.svc.t("cancel") : "Cancel"
    confirmText: root.svc ? root.svc.t("deleteProfile") : "Delete"
    foreground: root.fg
    onCanceled: root.profileRemoving = false
    onConfirmed: { root.profileRemoving = false; root.svc.deleteProfile(root.profileRemoveId) }
  }

  property string managerMissingText: root.svc && root.svc.managerInstalling ? root.svc.t("managerInstalling") : root.svc ? root.svc.t("managerMissingHint") : ""

  PageHeader {
    id: header
    anchors.left: parent.left; anchors.right: parent.right; anchors.top: parent.top
    title: root.svc ? root.svc.t("profilesTitle") : "Profiles"
    subtitle: root.svc && root.svc.managerInstalled ? root.svc.t("profilesCount", root.svc.profiles.length) : root.svc ? root.svc.t("managerMissing") : ""
    foreground: root.fg; fontFamily: root.fontFamily
    PanelActionButton {
      iconText: "󰐕"
      tooltipText: root.svc ? root.svc.t("addProfile") : "Add profile"
      foreground: root.fg
      hoverColor: Color.accent
      fontFamily: root.fontFamily
      enabled: root.svc && root.svc.managerInstalled && !root.svc.profileMutating
      onClicked: { addDialog.mode = "remote"; addDialog.open() }
    }
    PanelActionButton {
      iconText: "󰆍"
      tooltipText: root.svc ? root.svc.t("importCurrentConfig") : "Import current config"
      foreground: root.fg
      hoverColor: Color.accent
      fontFamily: root.fontFamily
      enabled: root.svc && root.svc.managerInstalled && root.svc.connected && !root.svc.profileMutating
      onClicked: { importDialog.open() }
    }
  }
  Rectangle { id: rule; anchors.left: parent.left; anchors.right: parent.right; anchors.top: header.bottom; height: 1; color: Util.alpha(root.fg,0.12) }
  Text {
    id: errorText
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: rule.bottom
    anchors.topMargin: Style.space(8)
    visible: root.svc && (root.svc.profileError !== "" || root.svc.profileMutating)
    text: root.svc && root.svc.profileMutating
      ? root.svc.t("profileActionInProgress")
      : root.svc ? root.svc.profileError : ""
    textFormat: Text.PlainText
    color: Color.urgent
    font.family: root.fontFamily
    font.pixelSize: Style.font.caption
    wrapMode: Text.WordWrap
  }
  ListView {
    id: list
    anchors.left: parent.left; anchors.right: parent.right; anchors.top: errorText.visible ? errorText.bottom : rule.bottom; anchors.bottom: parent.bottom
    anchors.topMargin: Style.space(14); clip: true; spacing: Style.space(8)
    model: root.svc ? root.svc.profiles : []
    delegate: Rectangle {
      required property var modelData
      width: ListView.view.width; height: Style.space(64); radius: Style.cornerRadius
      color: mouse.containsMouse ? Util.alpha(root.fg,0.07) : Util.alpha(root.fg,0.04)
      border.width: root.svc && root.svc.activeProfile === modelData.id ? 1 : 0
      border.color: Color.accent
      MouseArea { id: mouse; anchors.fill: parent; hoverEnabled: true; onClicked: if(root.svc) root.svc.selectProfile(modelData.id) }
      Column {
        anchors.left: parent.left; anchors.leftMargin: Style.space(12); anchors.right: actions.left; anchors.rightMargin: Style.space(8); anchors.verticalCenter: parent.verticalCenter; spacing: Style.space(3)
        Text { width: parent.width; text: (root.svc && root.svc.activeProfile === modelData.id ? "● " : "○ ") + modelData.name; color: root.svc && root.svc.activeProfile === modelData.id ? Color.accent : root.fg; font.family: root.fontFamily; font.pixelSize: Style.font.bodySmall; font.bold: true; elide: Text.ElideRight; renderType: Text.NativeRendering }
        Text { width: parent.width; text: String(modelData.type || "") + (modelData.lastSuccessAt ? " · " + modelData.lastSuccessAt : ""); color: Util.alpha(root.fg,0.5); font.family: root.fontFamily; font.pixelSize: Style.font.caption; elide: Text.ElideRight; renderType: Text.NativeRendering }
      }
      Row { id: actions; anchors.right: parent.right; anchors.rightMargin: Style.space(8); anchors.verticalCenter: parent.verticalCenter; spacing: Style.space(4)
        PanelActionButton { iconText: "󰑐"; tooltipText: root.svc ? root.svc.t("updateProfile") : "Update"; foreground: root.fg; hoverColor: Color.accent; fontFamily: root.fontFamily; enabled: root.svc && root.svc.managerInstalled && !root.svc.profileMutating; onClicked: root.svc.updateProfile(modelData.id,false) }
        PanelActionButton { iconText: "󰇧"; tooltipText: root.svc ? root.svc.t("updateViaProxy") : "Update via proxy"; foreground: root.fg; hoverColor: Color.accent; fontFamily: root.fontFamily; enabled: root.svc && root.svc.managerInstalled && root.svc.connected && !root.svc.profileMutating; onClicked: root.svc.updateProfileViaProxy(modelData.id) }
        PanelActionButton { iconText: "󰓝"; tooltipText: root.svc ? root.svc.t("renameProfile") : "Rename profile"; foreground: root.fg; hoverColor: Color.accent; fontFamily: root.fontFamily; enabled: root.svc && root.svc.managerInstalled && !root.svc.profileMutating; onClicked: { renameDialog.profileId = modelData.id; renameDialog.profileName = String(modelData.name || ""); renameDialog.open(); } }
        PanelActionButton { iconText: "󰖟"; tooltipText: root.svc ? root.svc.t("editProfileUrl") : "Edit profile URL"; foreground: root.fg; hoverColor: Color.accent; fontFamily: root.fontFamily; enabled: root.svc && root.svc.managerInstalled && modelData.type === "remote" && !root.svc.profileMutating; onClicked: { urlDialog.profileId = modelData.id; urlDialog.profileUrl = ""; root.svc.readProfileURL(modelData.id, function(value) { urlDialog.profileUrl = value; urlDialog.open(); }); } }
        PanelActionButton { iconText: "󰈙"; tooltipText: root.svc ? root.svc.t("profileSource") : "Source"; foreground: root.fg; hoverColor: Color.accent; fontFamily: root.fontFamily; enabled: root.svc && root.svc.managerInstalled && !root.svc.profileMutating; onClicked: { detail.profileId = modelData.id; detail.profileName = String(modelData.name || ""); detail.open() } }
        PanelActionButton { iconText: "󰅖"; tooltipText: root.svc ? root.svc.t("deleteProfile") : "Delete"; foreground: root.fg; hoverColor: Color.urgent; fontFamily: root.fontFamily; enabled: root.svc && !root.svc.profileMutating; onClicked: { root.profileRemoveId = modelData.id; root.profileRemoveName = String(modelData.name || ""); root.profileRemoving = true } }
      }
    }
  }
  Text { anchors.centerIn: list; width: list.width-Style.space(40); visible: list.count === 0; text: root.svc && !root.svc.managerInstalled ? root.managerMissingText : root.svc ? root.svc.t("noProfiles") : ""; color: Util.alpha(root.fg,0.5); font.family: root.fontFamily; font.pixelSize: Style.font.bodySmall; horizontalAlignment: Text.AlignHCenter; wrapMode: Text.WordWrap; renderType: Text.NativeRendering }
  Button { anchors.horizontalCenter: list.horizontalCenter; anchors.top: list.verticalCenter; visible: root.svc && !root.svc.managerInstalled; text: root.svc && root.svc.managerInstalling ? root.svc.t("installing") : root.svc ? root.svc.t("installManager") : "Install"; enabled: root.svc && !root.svc.managerInstalling; onClicked: root.svc.installManager() }

}
