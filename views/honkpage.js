var csrftoken = ""
var honksforpage = { }
var curpagestate = { name: "", arg : "" }
var tophid = { }
var servermsgs = { }

function encode(hash) {
	var s = []
	for (var key in hash) {
		var val = hash[key]
		s.push(encodeURIComponent(key) + "=" + encodeURIComponent(val))
	}
	return s.join("&")
}
function post(url, data) {
	var x = new XMLHttpRequest()
	x.open("POST", url)
	x.timeout = 30 * 1000
	x.setRequestHeader("Content-Type", "application/x-www-form-urlencoded")
	x.send(data)
}
function get(url, whendone, errfunction) {
	var x = new XMLHttpRequest()
	x.open("GET", url)
	x.timeout = 15 * 1000
	x.responseType = "json"
	x.onload = function() { whendone(x) }
	if (errfunction) {
		x.ontimeout = function(e) { errfunction(" timed out") }
		x.onerror = function(e) { errfunction(" error") }
	}
	x.send()
}
function bonk(el, xid) {
	el.innerHTML = "bonked"
	el.disabled = true
	post("/bonk", encode({"js": "2", "CSRF": csrftoken, "xid": xid}))
	return false
}
function unbonk(el, xid) {
	el.innerHTML = "unbonked"
	el.disabled = true
	post("/zonkit", encode({"CSRF": csrftoken, "wherefore": "unbonk", "what": xid}))
}
function muteit(el, convoy) {
	el.innerHTML = "muted"
	el.disabled = true
	post("/zonkit", encode({"CSRF": csrftoken, "wherefore": "zonvoy", "what": convoy}))
	var els = document.querySelectorAll('article.honk')
	for (var i = 0; i < els.length; i++) {
		var e = els[i]
		if (e.getAttribute("data-convoy") == convoy) {
			e.remove()
		}
	}
}
function zonkit(el, xid) {
	el.innerHTML = "zonked"
	el.disabled = true
	post("/zonkit", encode({"CSRF": csrftoken, "wherefore": "zonk", "what": xid}))
	var p = el
	while (p && p.tagName != "ARTICLE") {
		p = p.parentElement
	}
	if (p) {
		p.remove()
	}
}
function flogit(el, how, xid) {
	var s = how
	if (s[s.length-1] != "e") { s += "e" }
	s += "d"
	if (s == "untaged") s = "untagged"
	if (s == "reacted") s = "badonked"
	el.innerHTML = s
	el.disabled = true
	post("/zonkit", encode({"CSRF": csrftoken, "wherefore": how, "what": xid}))
}

var lehonkform = document.getElementById("honkform")
var lehonkbutton = document.getElementById("honkingtime")

function oldestnewest(btn) {
	var els = document.getElementsByClassName("glow")
	if (els.length) {
		els[els.length-1].scrollIntoView({ behavior: "smooth" })
	}
}
function removeglow() {
	var els = document.getElementsByClassName("glow")
	while (els.length) {
		els[0].classList.remove("glow")
	}
}

function fillinhonks(xhr, glowit) {
	var resp = xhr.response
	var stash = curpagestate.name + ":" + curpagestate.arg
	tophid[stash] = resp.Tophid
	var doc = document.createElement( 'div' );
	doc.innerHTML = resp.Srvmsg
	var srvmsg = doc
	doc = document.createElement( 'div' );
	doc.innerHTML = resp.Honks
	var honks = doc.children

	var mecount = document.getElementById("mecount")
	if (resp.MeCount) {
		mecount.innerHTML = "(" + resp.MeCount + ")"
	} else {
		mecount.innerHTML = ""
	}
	var chatcount = document.getElementById("chatcount")
	if (resp.ChatCount) {
		chatcount.innerHTML = "(" + resp.ChatCount + ")"
	} else {
		chatcount.innerHTML = ""
	}

	var srvel = document.getElementById("srvmsg")
	while (srvel.children[0]) {
		srvel.children[0].remove()
	}
	srvel.prepend(srvmsg)

	var frontload = true
	if (curpagestate.name == "convoy") {
		frontload = false
	}

	var honksonpage = document.getElementById("honksonpage")
	var holder = honksonpage.children[0]
	var lenhonks = honks.length
	for (var i = honks.length; i > 0; i--) {
		var h = honks[frontload ? i-1 : 0]
		if (glowit)
			h.classList.add("glow")
		if (frontload) {
			holder.prepend(h)
		} else {
			holder.append(h)
		}
	}
	relinklinks()
	return lenhonks
}
function hydrargs() {
	var name = curpagestate.name
	var arg = curpagestate.arg
	var args = { "page" : name }
	if (name == "convoy") {
		args["c"] = arg
	} else if (name == "combo") {
		args["c"] = arg
	} else if (name == "honker") {
		args["xid"] = arg
	} else if (name == "user") {
		args["uname"] = arg
	}
	return args
}
function refreshupdate(msg) {
	var el = document.querySelector("#refreshbox p span")
	if (el) {
		el.innerHTML = msg
	}
}
function refreshhonks(btn) {
	removeglow()
	btn.innerHTML = "refreshing"
	btn.disabled = true
	var args = hydrargs()
	var stash = curpagestate.name + ":" + curpagestate.arg
	args["tophid"] = tophid[stash]
	get("/hydra?" + encode(args), function(xhr) {
		btn.innerHTML = "refresh"
		btn.disabled = false
		if (xhr.status == 200) {
			var lenhonks = fillinhonks(xhr, true)
			refreshupdate(" " + lenhonks + " new")
		} else {
			refreshupdate(" status: " + xhr.status)
		}
	}, function(err) {
		btn.innerHTML = "refresh"
		btn.disabled = false
		refreshupdate(err)
	})
}
function statechanger(evt) {
	var data = evt.state
	if (!data) {
		return
	}
	switchtopage(data.name, data.arg)
}
function switchtopage(name, arg, anchor) {
	var stash = curpagestate.name + ":" + curpagestate.arg
	var honksonpage = document.getElementById("honksonpage")
	var holder = honksonpage.children[0]
	holder.remove()
	var srvel = document.getElementById("srvmsg")
	var msg = srvel.children[0]
	if (msg) {
		msg.remove()
		servermsgs[stash] = msg
	}
	showelement("refreshbox")

	honksforpage[stash] = holder

	curpagestate.name = name
	curpagestate.arg = arg
	// get the holder for the target page
	stash = name + ":" + arg
	holder = honksforpage[stash]
	if (holder) {
		honksonpage.prepend(holder)
		msg = servermsgs[stash]
		if (msg) {
			srvel.prepend(msg)
		}
		if (anchor) {
			let el = document.getElementById(anchor)
			el.scrollIntoView()
		}
	} else {
		// or create one and fill it
		honksonpage.prepend(document.createElement("div"))
		var args = hydrargs()
		get("/hydra?" + encode(args), function(xhr) {
			if (xhr.status == 200) {
				fillinhonks(xhr, false)
				if (anchor) {
					let el = document.getElementById(anchor)
					el.scrollIntoView()
				}
			} else {
				refreshupdate(" status: " + xhr.status)
			}
		}, function(err) {
			refreshupdate(err)
		})
	}
	refreshupdate("")
}
function newpagestate(name, arg) {
	return { "name": name, "arg": arg }
}
function pageswitcher(name, arg) {
	return function(evt) {
		var topmenu = document.getElementById("topmenu")
		topmenu.open = false
		if (name == curpagestate.name && arg == curpagestate.arg) {
			return false
		}
		let url = evt.srcElement.href
		if (!url)
			url = evt.srcElement.parentElement.href
		let anchor
		let arr = url.split("#")
		if (arr.length == 2)
			anchor = arr[1]
		switchtopage(name, arg, anchor)
		history.pushState(newpagestate(name, arg), "some title", url)
		window.scrollTo(0, 0)
		return false
	}
}
function relinklinks() {
	var els = document.getElementsByClassName("convoylink")
	while (els.length) {
		els[0].onclick = pageswitcher("convoy", els[0].text)
		els[0].classList.remove("convoylink")
	}
	els = document.getElementsByClassName("combolink")
	while (els.length) {
		els[0].onclick = pageswitcher("combo", els[0].text)
		els[0].classList.remove("combolink")
	}
	els = document.getElementsByClassName("honkerlink")
	while (els.length) {
		var el = els[0]
		var xid = el.getAttribute("data-xid")
		el.onclick = pageswitcher("honker", xid)
		el.classList.remove("honkerlink")
	}
	els = document.getElementsByClassName("donklink")
	while (els.length) {
		let el = els[0]
		el.onclick = function() {
			el.classList.remove("donk")
			el.onclick = null
			return false
		}
		el.classList.remove("donklink")
	}

	els = document.querySelectorAll("#honksonpage article button")
	els.forEach(function(el) {
		var honk = el.closest("article")
		var convoy = honk.dataset.convoy
		var hname = honk.dataset.hname
		var xid = honk.dataset.xid
		var id = Number(honk.dataset.id)

		if (!(id > 0)) {
			console.error("could not determine honk id")
			return
		}

		if (el.classList.contains("unbonk")) {
			el.onclick = function() {
				unbonk(el, xid);
			}
		} else if (el.classList.contains("bonk")) {
			el.onclick = function() {
				bonk(el, xid)
			}
		} else if (el.classList.contains("honkback")) {
			el.onclick = function() {
				return showhonkform(el, xid, hname)
			}
		} else if (el.classList.contains("mute")) {
			el.onclick = function() {
				muteit(el, convoy);
			}
		} else if (el.classList.contains("evenmore")) {
			var more = document.querySelector("#evenmore"+id);
			el.onclick = function() {
				more.classList.toggle("hide");
			}
		} else if (el.classList.contains("zonk")) {
			el.onclick = function() {
				zonkit(el, xid);
			}
		} else if (el.classList.contains("flogit-deack")) {
			el.onclick = function() {
				flogit(el, "deack", xid);
			}
		} else if (el.classList.contains("flogit-ack")) {
			el.onclick = function() {
				flogit(el, "ack", xid);
			}
		} else if (el.classList.contains("flogit-unsave")) {
			el.onclick = function() {
				flogit(el, "unsave", xid);
			}
		} else if (el.classList.contains("flogit-save")) {
			el.onclick = function() {
				flogit(el, "save", xid);
			}
		} else if (el.classList.contains("flogit-untag")) {
			el.onclick = function() {
				flogit(el, "untag", xid);
			}
		} else if (el.classList.contains("flogit-react")) {
			el.onclick = function() {
				flogit(el, "react", xid);
			}
		}
	})
}
function showhonkform(elem, rid, hname) {
	var form = lehonkform
	form.style = "display: block"
	form.reset()
	if (elem) {
		form.remove()
		elem.parentElement.parentElement.parentElement.insertAdjacentElement('beforebegin', form)
	} else {
		hideelement(lehonkbutton)
		elem = document.getElementById("honkformhost")
		elem.insertAdjacentElement('afterend', form)
	}
	var donker = document.getElementById("donker")
	donker.children[1].textContent = ""
	var ridinput = document.getElementById("ridinput")
	var honknoise = document.getElementById("honknoise")
	if (rid) {
		ridinput.value = rid
		if (hname) {
			honknoise.value = hname + " "
		} else {
			honknoise.value = ""
		}
	} else {
		ridinput.value = ""
		honknoise.value = ""
	}
	honknoise.ondrop = donkdrop
	var updateinput = document.getElementById("updatexidinput")
	updateinput.value = ""
	var savedfile = document.getElementById("saveddonkxid")
	savedfile.value = ""
	honknoise.focus()
	return false
}
function donkdrop(evt) {
	evt.preventDefault()
	let donks = document.querySelector("#donker input")
	Array.from(evt.dataTransfer.items).forEach((item) => {
        if (item.kind == "file") {
			let olddonks = donks.files
			let donkarama = new DataTransfer();
			for (donk of olddonks)
				donkarama.items.add(donk)
            let file = item.getAsFile()
			donkarama.items.add(file)
			donks.files = donkarama.files;
            let t = evt.target
            let start = t.selectionStart
            let s = t.value.substr(0, start)
            let e = t.value.substr(start)
            t.value = s + `<img src=${donks.files.length}>` + e
            t.selectionStart = start
            t.selectionEnd = start
        }
    })
}
function cancelhonking() {
	hideelement(lehonkform)
	showelement(lehonkbutton)
}
function showelement(el) {
	if (typeof(el) == "string")
		el = document.getElementById(el)
	if (!el) return
	el.style.display = "block"
}
function hideelement(el) {
	if (typeof(el) == "string")
		el = document.getElementById(el)
	if (!el) return
	el.style.display = "none"
}
function updatedonker(ev) {
	var el = ev.target.parentElement
	el.children[1].textContent = el.children[0].value.slice(-20)
	el = el.nextSibling
	el.value = ""
	el = el.parentElement.nextSibling
	el.style.display = ""
}
var checkinprec = 100.0
var gpsoptions = {
	enableHighAccuracy: false,
	timeout: 1000,
	maximumAge: 0
};
function fillcheckin() {
	if (navigator.geolocation) {
		navigator.geolocation.getCurrentPosition(function(pos) {
			showelement("placedescriptor")
			var el = document.getElementById("placelatinput")
			el.value = Math.round(pos.coords.latitude * checkinprec) / checkinprec
			el = document.getElementById("placelonginput")
			el.value = Math.round(pos.coords.longitude * checkinprec) / checkinprec
			checkinprec = 10000.0
			gpsoptions.enableHighAccuracy = true
			gpsoptions.timeout = 2000
		}, function(err) {
			showelement("placedescriptor")
			var el = document.getElementById("placenameinput")
			el.value = err.message
		}, gpsoptions)
	}
}

function scrollnexthonk() {
	var honks = document.getElementsByClassName("honk");
	for (var i = 0; i < honks.length; i++) {
		var h = honks[i];
		var b = h.getBoundingClientRect();
		if (b.top > 1.0) {
			h.scrollIntoView()
			var a = h.querySelector(".actions summary")
			if (a) a.focus({ preventScroll: true })
			break
		}
	}
}

function scrollprevioushonk() {
	var honks = document.getElementsByClassName("honk");
	for (var i = 1; i < honks.length; i++) {
		var b = honks[i].getBoundingClientRect();
		if (b.top > -1.0) {
			honks[i-1].scrollIntoView()
			var a = honks[i-1].querySelector(".actions summary")
			if (a) a.focus({ preventScroll: true })
			break
		}
	}
}

function addemu(elem) {
	const data = elem.alt
	const box = document.getElementById("honknoise");
	box.value += data;
}
function loademus() {
	var div = document.getElementById("emupicker")
	var request = new XMLHttpRequest()
	request.open('GET', '/emus')
	request.onload = function() {
		div.innerHTML = request.responseText
		div.querySelectorAll(".emu").forEach(function(el) {
			el.onclick = function() {
				addemu(el)
			}
		})
	}
	if (div.style.display === "none") {
		div.style.display = "block";
	} else {
		div.style.display = "none";
	}
	request.send()
}

// init
(function() {
	var me = document.currentScript;
	csrftoken = me.dataset.csrf
	curpagestate.name = me.dataset.pagename
	curpagestate.arg = me.dataset.pagearg
	tophid[curpagestate.name + ":" + curpagestate.arg] = me.dataset.tophid
	servermsgs[curpagestate.name + ":" + curpagestate.arg] = me.dataset.srvmsg

	var el = document.getElementById("homelink")
	el.onclick = pageswitcher("home", "")
	el = document.getElementById("atmelink")
	el.onclick = pageswitcher("atme", "")
	el = document.getElementById("firstlink")
	el.onclick = pageswitcher("first", "")
	el = document.getElementById("savedlink")
	el.onclick = pageswitcher("saved", "")
	el = document.getElementById("longagolink")
	el.onclick = pageswitcher("longago", "")

	var refreshbox = document.getElementById("refreshbox")
	if (refreshbox) {
		refreshbox.querySelectorAll("button").forEach(function(el) {
			if (el.classList.contains("refresh")) {
				el.onclick = function() {
					refreshhonks(el)
				}
			} else if (el.classList.contains("scrolldown")) {
				el.onclick = function() {
					oldestnewest(el)
				}
			}
		})

		if (me.dataset.srvmsg == "one honk maybe more") {
			hideelement(refreshbox)
		}
	}

	var td = document.getElementById("timedescriptor")
	document.getElementById("addtimebutton").onclick = function() {
		td.classList.toggle("hide")
	}
	document.getElementById("honkingtime").onclick = function() {
		return showhonkform()
	}
	document.getElementById("checkinbutton").onclick = fillcheckin
	document.getElementById("emuload").onclick = loademus
	document.querySelector("#donker input").onchange = updatedonker
	document.querySelector("button[name=cancel]").onclick = cancelhonking

	relinklinks()
	window.onpopstate = statechanger
	history.replaceState(curpagestate, "some title", "")

	hideelement("donkdescriptor")
})();
