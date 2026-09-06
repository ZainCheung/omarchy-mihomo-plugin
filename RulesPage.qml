import QtQuick
import QtQuick.Controls
import qs.Ui
import qs.Commons

Item {
  id: root

  property var svc: null
  property color fg: Color.popups.text
  property string fontFamily: Style.font.family
  property string filter: ""
  property string editingRuleId: ""
  property string removingRuleId: ""
  property bool removingRule: false

  readonly property var effectiveRows: {
    if (!root.svc || root.filter.trim() === "") return []
    var needle = root.filter.trim().toLowerCase()
    var all = root.svc.rules || []
    var out = []
    for (var i = 0; i < all.length; i++) {
      var rule = all[i] || {}
      var payload = String(rule.payload || rule.rulePayload || "")
      var type = String(rule.type || "")
      var target = String(rule.proxy || rule.policy || "")
      if (payload.toLowerCase().indexOf(needle) >= 0
          || type.toLowerCase().indexOf(needle) >= 0
          || target.toLowerCase().indexOf(needle) >= 0) {
        out.push({index: i, payload: payload, type: type, target: target})
      }
    }
    return out
  }

  function ruleDomain(rule) {
    var match = rule && rule.match ? rule.match : {}
    return String(match.value || "")
  }

  function ruleMatch(rule) {
    var match = rule && rule.match ? rule.match : {}
    return String(match.type || "domain-suffix")
  }

  function matchLabel(rule) {
    return root.svc
      ? root.svc.t(root.ruleMatch(rule) === "domain" ? "matchDomain" : "matchDomainSuffix")
      : (root.ruleMatch(rule) === "domain" ? "Exact domain" : "Domain and subdomains")
  }

  function policyLabel(policy) {
    if (!root.svc) return String(policy || "")
    if (policy === "direct") return root.svc.t("policyDirect")
    if (policy === "reject") return root.svc.t("policyReject")
    return root.svc.t("policyProxy")
  }

  function policyTarget(rule) {
    if (!rule) return ""
    if (rule.policy === "direct") return "DIRECT"
    if (rule.policy === "reject") return "REJECT"
    return root.svc ? root.svc.t("policyProxy") : "Proxy"
  }

  function targetColor(target) {
    if (target === "DIRECT") return Util.alpha(root.fg, 0.6)
    if (target === "REJECT" || target === "REJECT-DROP") return Color.urgent
    return Color.accent
  }

  function openAdd() {
    addRuleDialog.ruleId = ""
    addRuleDialog.initialDomain = ""
    addRuleDialog.initialMatch = "domain-suffix"
    addRuleDialog.initialPolicy = "proxy"
    addRuleDialog.open()
  }

  function openEdit(rule) {
    editRuleDialog.ruleId = String(rule.id || "")
    editRuleDialog.initialDomain = root.ruleDomain(rule)
    editRuleDialog.initialMatch = root.ruleMatch(rule)
    editRuleDialog.initialPolicy = String(rule.policy || "proxy")
    editRuleDialog.open()
  }

  function removeRule(rule) {
    root.removingRuleId = String(rule.id || "")
    removeConfirm.message = root.svc
      ? root.svc.t("deleteRuleConfirm", root.ruleDomain(rule))
      : "Delete this rule?"
    root.removingRule = true
  }

  function scrollBy(delta) {
    var flick = scrollView.contentItem
    if (!flick) return
    flick.contentY = Math.max(0, Math.min(Math.max(0, flick.contentHeight - flick.height),
                                          flick.contentY + delta))
  }

  function focusFilter() {
    filterField.forceActiveFocus()
    filterField.selectAll()
  }

  AddRuleDialog {
    id: addRuleDialog
    svc: root.svc
    foreground: root.fg
    fontFamily: root.fontFamily
  }

  AddRuleDialog {
    id: editRuleDialog
    svc: root.svc
    foreground: root.fg
    fontFamily: root.fontFamily
  }

  ConfirmDialog {
    id: removeConfirm
    anchors.fill: parent
    z: 100
    opened: root.removingRule
    cancelText: root.svc ? root.svc.t("cancel") : "Cancel"
    confirmText: root.svc ? root.svc.t("deleteRule") : "Delete rule"
    foreground: root.fg
    onCanceled: {
      root.removingRule = false
      root.removingRuleId = ""
    }
    onConfirmed: {
      if (root.svc && root.removingRuleId !== "") root.svc.deleteCustomRule(root.removingRuleId)
      root.removingRule = false
      root.removingRuleId = ""
    }
  }

  PageHeader {
    id: header
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: parent.top
    title: root.svc ? root.svc.t("rulesTitle") : "Rules"
    subtitle: root.svc ? root.svc.t("globalRulesSubtitle") : "Rules used by every profile"
    foreground: root.fg
    fontFamily: root.fontFamily

    PanelActionButton {
      iconText: "󰐕"
      tooltipText: root.svc ? root.svc.t("addRule") : "Add rule"
      foreground: root.fg
      hoverColor: Color.accent
      fontFamily: root.fontFamily
      enabled: root.svc && root.svc.managerInstalled && !root.svc.profileMutating
      onClicked: root.openAdd()
    }

    PanelActionButton {
      iconText: "󰑐"
      tooltipText: root.svc ? root.svc.t("refreshRules") : "Reload rules"
      foreground: root.fg
      hoverColor: Color.accent
      fontFamily: root.fontFamily
      enabled: root.svc && root.svc.connected && !root.svc.rulesLoading
      onClicked: {
        root.svc.refreshRules()
        root.svc.refreshCustomRules()
      }
    }
  }

  Rectangle {
    id: headerRule
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: header.bottom
    height: 1
    color: Util.alpha(root.fg, 0.12)
  }

  ScrollView {
    id: scrollView
    anchors.left: parent.left
    anchors.right: parent.right
    anchors.top: headerRule.bottom
    anchors.bottom: parent.bottom
    anchors.topMargin: Style.space(14)
    clip: true
    ScrollBar.vertical.policy: ScrollBar.AsNeeded

    Column {
      id: content
      width: scrollView.availableWidth
      spacing: Style.space(10)

      Card {
        id: myRulesCard
        width: content.width
        foreground: root.fg

        Row {
          width: parent.width
          implicitHeight: Math.max(myRulesHeader.implicitHeight, addButton.implicitHeight)

          PanelSectionHeader {
            id: myRulesHeader
            width: parent.width - addButton.width - Style.space(10)
            text: root.svc ? root.svc.t("myRules") : "My Rules"
            foreground: root.fg
            fontFamily: root.fontFamily
          }

          Button {
            id: addButton
            anchors.verticalCenter: parent.verticalCenter
            text: root.svc ? root.svc.t("addRule") : "Add rule"
            enabled: root.svc && root.svc.managerInstalled && !root.svc.profileMutating
            onClicked: root.openAdd()
          }
        }

        Repeater {
          model: root.svc ? root.svc.customRules : []

          delegate: Rectangle {
            required property var modelData
            width: myRulesCard.width - myRulesCard.pad * 2
            height: Style.space(54)
            radius: Style.cornerRadius
            color: ruleMouse.containsMouse ? Util.alpha(root.fg, 0.07) : Util.alpha(root.fg, 0.03)

            MouseArea {
              id: ruleMouse
              anchors.fill: parent
              hoverEnabled: true
            }

            Column {
              anchors.left: parent.left
              anchors.right: ruleActions.left
              anchors.leftMargin: Style.space(10)
              anchors.rightMargin: Style.space(8)
              anchors.verticalCenter: parent.verticalCenter
              spacing: Style.space(2)

              Text {
                width: parent.width
                text: root.ruleDomain(modelData)
                textFormat: Text.PlainText
                color: modelData.enabled === false ? Util.alpha(root.fg, 0.45) : root.fg
                font.family: root.fontFamily
                font.pixelSize: Style.font.bodySmall
                font.bold: true
                elide: Text.ElideRight
                renderType: Text.NativeRendering
              }

              Text {
                width: parent.width
                text: root.policyLabel(String(modelData.policy || "proxy"))
                  + " · " + root.matchLabel(modelData)
                textFormat: Text.PlainText
                color: modelData.enabled === false ? Util.alpha(root.fg, 0.35) : Util.alpha(root.fg, 0.55)
                font.family: root.fontFamily
                font.pixelSize: Style.font.caption
                elide: Text.ElideRight
                renderType: Text.NativeRendering
              }
            }

            Row {
              id: ruleActions
              anchors.right: parent.right
              anchors.rightMargin: Style.space(8)
              anchors.verticalCenter: parent.verticalCenter
              spacing: Style.space(2)

              PanelActionButton {
                iconText: "󰏫"
                tooltipText: root.svc ? root.svc.t("editRule") : "Edit rule"
                foreground: root.fg
                hoverColor: Color.accent
                fontFamily: root.fontFamily
                enabled: root.svc && !root.svc.profileMutating
                onClicked: root.openEdit(modelData)
              }

              PanelActionButton {
                iconText: modelData.enabled === false ? "󰐊" : "󰏤"
                tooltipText: root.svc
                  ? root.svc.t(modelData.enabled === false ? "enableRule" : "disableRule")
                  : (modelData.enabled === false ? "Enable" : "Disable")
                foreground: root.fg
                hoverColor: Color.accent
                fontFamily: root.fontFamily
                enabled: root.svc && !root.svc.profileMutating
                onClicked: modelData.enabled === false
                  ? root.svc.enableCustomRule(String(modelData.id))
                  : root.svc.disableCustomRule(String(modelData.id))
              }

              PanelActionButton {
                iconText: "󰆴"
                tooltipText: root.svc ? root.svc.t("deleteRule") : "Delete rule"
                foreground: root.fg
                hoverColor: Color.urgent
                fontFamily: root.fontFamily
                enabled: root.svc && !root.svc.profileMutating
                onClicked: root.removeRule(modelData)
              }
            }
          }
        }

        Text {
          width: parent.width
          visible: root.svc && root.svc.customRules.length === 0
          text: root.svc ? root.svc.t("noCustomRules") : "No custom rules yet."
          textFormat: Text.PlainText
          color: Util.alpha(root.fg, 0.5)
          font.family: root.fontFamily
          font.pixelSize: Style.font.bodySmall
          renderType: Text.NativeRendering
        }
      }

      Card {
        id: effectiveRulesCard
        width: content.width
        foreground: root.fg

        PanelSectionHeader {
          width: parent.width
          text: root.svc ? root.svc.t("effectiveRules") : "Effective Rules"
          foreground: root.fg
          fontFamily: root.fontFamily
        }

        TextField {
          id: filterField
          width: parent.width
          placeholderText: root.svc ? root.svc.t("searchEffectiveRulesHint") : "Search the active configuration's rules"
          foreground: root.fg
          accent: Color.accent
          font.family: root.fontFamily
          font.pixelSize: Style.font.bodySmall
          onTextChanged: root.filter = text
          Keys.onEscapePressed: function(event) {
            if (text !== "") { text = ""; event.accepted = true }
            else focus = false
          }
        }

        Text {
          width: parent.width
          visible: root.filter.trim() === ""
          text: root.svc ? root.svc.t("searchEffectiveRules") : "Search to inspect effective rules."
          textFormat: Text.PlainText
          color: Util.alpha(root.fg, 0.5)
          font.family: root.fontFamily
          font.pixelSize: Style.font.caption
          wrapMode: Text.WordWrap
          renderType: Text.NativeRendering
        }

        Repeater {
          model: root.effectiveRows

          delegate: Rectangle {
            required property var modelData
            width: effectiveRulesCard.width - effectiveRulesCard.pad * 2
            height: Style.space(38)
            radius: Style.cornerRadius
            color: Util.alpha(root.fg, 0.03)

            Text {
              anchors.left: parent.left
              anchors.leftMargin: Style.space(8)
              anchors.right: target.left
              anchors.rightMargin: Style.space(8)
              anchors.verticalCenter: parent.verticalCenter
              text: String(modelData.index + 1) + "  " + modelData.payload + " · " + modelData.type
              textFormat: Text.PlainText
              color: root.fg
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
              elide: Text.ElideMiddle
              renderType: Text.NativeRendering
            }

            Text {
              id: target
              anchors.right: parent.right
              anchors.rightMargin: Style.space(8)
              anchors.verticalCenter: parent.verticalCenter
              width: Style.space(110)
              text: modelData.target
              textFormat: Text.PlainText
              color: root.targetColor(modelData.target)
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
              font.bold: true
              horizontalAlignment: Text.AlignRight
              elide: Text.ElideRight
              renderType: Text.NativeRendering
            }
          }
        }

        Text {
          width: parent.width
          visible: root.filter.trim() !== "" && root.effectiveRows.length === 0
          text: root.svc ? root.svc.t("noMatchRules") : "No rules match."
          textFormat: Text.PlainText
          color: Util.alpha(root.fg, 0.5)
          font.family: root.fontFamily
          font.pixelSize: Style.font.bodySmall
          renderType: Text.NativeRendering
        }
      }

      Item { width: 1; height: Style.space(4) }
    }
  }
}
